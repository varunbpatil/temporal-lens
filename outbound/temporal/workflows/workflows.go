// Package workflows implements direct gRPC access to Temporal's WorkflowService.
package workflows

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"math"
	"net/url"
	"os"
	"path"
	"slices"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	batchpb "go.temporal.io/api/batch/v1"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	failurepb "go.temporal.io/api/failure/v1"
	filterpb "go.temporal.io/api/filter/v1"
	historypb "go.temporal.io/api/history/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/varunbpatil/temporal-lens/config"
	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/types"
)

const (
	// metadataPageSize bounds the number of workflows returned by the list API response.
	metadataPageSize = 2_000

	// workflowDataPageSize bounds the amount of event history held per response.
	workflowDataPageSize = 1_000

	// clientIdentity is the client identity for bulk actions. This shows up in the Temporal UI.
	clientIdentity = "temporal-lens"
)

var _ ports.WorkflowSource = (*Source)(nil)

// Source has an independent connection pool per namespace. This is required
// by Temporal Cloud, whose endpoint is namespace-specific.
type Source struct {
	// Temporal UI base URL
	baseURL *url.URL

	// Connection pool per Temporal namespace
	pools map[string]*clientPool
}

// clientPool is a round-robin connection pool per Temporal namespace.
type clientPool struct {
	clients []workflowservice.WorkflowServiceClient
	conns   []*grpc.ClientConn

	// bulkActionsPerSecond is passed to Temporal's native batch-operation worker.
	bulkActionsPerSecond float32

	metadataLimiter *rate.Limiter
	dataLimiter     *rate.Limiter

	// next is atomic because sources are used concurrently by request handlers.
	next atomic.Uint64
}

type WorkflowSourceParams struct {
	Config      config.TemporalConfig
	DialOptions []grpc.DialOption
}

// NewSource creates all namespace clients concurrently.
func NewSource(ctx context.Context, params WorkflowSourceParams) (*Source, error) {
	cfg := params.Config
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	s := &Source{
		baseURL: &cfg.BaseURL,
		pools:   make(map[string]*clientPool, len(cfg.Namespaces)),
	}
	for _, namespace := range cfg.Namespaces {
		s.pools[namespace] = &clientPool{
			clients:              make([]workflowservice.WorkflowServiceClient, cfg.ConnPoolSize),
			conns:                make([]*grpc.ClientConn, cfg.ConnPoolSize),
			bulkActionsPerSecond: float32(cfg.BulkActionsPerSecond),
			metadataLimiter:      rate.NewLimiter(rate.Limit(cfg.ListRequestsPerSecond), 1),
			dataLimiter:          rate.NewLimiter(rate.Limit(cfg.HistoryRequestsPerSecond), 1),
		}
	}
	options, err := dialOptions(cfg)
	if err != nil {
		return nil, err
	}
	options = append(options, params.DialOptions...)
	if clientErr := s.createClients(ctx, cfg, options); clientErr != nil {
		_ = s.Close()
		return nil, clientErr
	}
	return s, nil
}

// validateConfig rejects configuration that cannot produce a usable source.
func validateConfig(cfg config.TemporalConfig) error {
	if len(cfg.Namespaces) == 0 ||
		cfg.ConnPoolSize < 1 ||
		cfg.BulkActionsPerSecond < 1 ||
		cfg.ListRequestsPerSecond < 1 ||
		cfg.HistoryRequestsPerSecond < 1 {
		return errors.New(
			"temporal namespaces, positive connection pool size, and positive actions and stream requests per second are required",
		)
	}
	if cfg.Cloud && cfg.Account == "" {
		return errors.New("temporal account is required for Temporal Cloud")
	}
	if !cfg.Cloud && cfg.Endpoint == "" {
		return errors.New("temporal endpoint is required for self-hosted Temporal")
	}
	if cfg.BaseURL.Scheme == "" || cfg.BaseURL.Host == "" {
		return errors.New("temporal base URL must be absolute")
	}
	return nil
}

// createClients populates every configured namespace connection pool.
func (s *Source) createClients(ctx context.Context, cfg config.TemporalConfig, options []grpc.DialOption) error {
	// Establish every connection in parallel; a failure cancels the remaining work.
	group, groupCtx := errgroup.WithContext(ctx)
	for _, namespace := range cfg.Namespaces {
		pool := s.pools[namespace]
		for index := range pool.conns {
			group.Go(func() error {
				conn, dialErr := grpc.NewClient(endpointFor(cfg, namespace), options...)
				if dialErr != nil {
					return fmt.Errorf("create Temporal client %d for %q: %w", index, namespace, dialErr)
				}
				if contextErr := groupCtx.Err(); contextErr != nil {
					_ = conn.Close()
					return contextErr
				}
				pool.conns[index] = conn
				pool.clients[index] = workflowservice.NewWorkflowServiceClient(conn)
				return nil
			})
		}
	}
	if groupErr := group.Wait(); groupErr != nil {
		return groupErr
	}
	return nil
}

// Close releases the adapter's gRPC connections during application shutdown.
func (s *Source) Close() error {
	errs := make([]error, 0)
	for _, pool := range s.pools {
		for _, conn := range pool.conns {
			if conn != nil {
				errs = append(errs, conn.Close())
			}
		}
	}
	return errors.Join(errs...)
}

// endpointFor selects the namespace-specific Cloud endpoint or shared self-hosted endpoint.
func endpointFor(cfg config.TemporalConfig, namespace string) string {
	if cfg.Cloud {
		// Temporal Cloud addresses each namespace at its own account-qualified endpoint.
		return namespace + "." + cfg.Account + ".tmprl.cloud:7233"
	}
	return cfg.Endpoint
}

// dialOptions builds transport security and optional API-key authentication for gRPC clients.
func dialOptions(cfg config.TemporalConfig) ([]grpc.DialOption, error) {
	var creds credentials.TransportCredentials
	if cfg.Cloud || cfg.TLS {
		tlsConfig, err := newTLSConfig(cfg)
		if err != nil {
			return nil, err
		}
		creds = credentials.NewTLS(tlsConfig)
	} else {
		creds = insecure.NewCredentials()
	}
	opts := []grpc.DialOption{grpc.WithTransportCredentials(creds)}
	if cfg.APIKey != "" {
		// The Cloud API accepts an API key as a bearer authorization header.
		opts = append(opts, grpc.WithUnaryInterceptor(apiKeyInterceptor(cfg.APIKey)))
	}
	return opts, nil
}

// newTLSConfig loads the configured CA and optional client certificate material.
func newTLSConfig(cfg config.TemporalConfig) (*tls.Config, error) {
	// Cloud always requires TLS; self-hosted deployments opt in with cfg.TLS.
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         cfg.ServerName,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	} // #nosec G402 -- operator-configured.
	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read Temporal CA file: %w", err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("parse Temporal CA file")
		}
		tlsConfig.RootCAs = roots
	}
	if (cfg.ClientCertFile == "") != (cfg.ClientKeyFile == "") {
		return nil, errors.New("temporal client certificate and key must be configured together")
	}
	if cfg.ClientCertFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCertFile, cfg.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load Temporal mTLS certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return tlsConfig, nil
}

// apiKeyInterceptor attaches the configured API key to every unary RPC.
func apiKeyInterceptor(key string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, conn *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoke(
			metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+key),
			method,
			req,
			reply,
			conn,
			opts...)
	}
}

// clientPool returns the connection pool and shared rate limits for a namespace.
func (s *Source) clientPool(namespace string) (*clientPool, error) {
	pool, ok := s.pools[namespace]
	if !ok {
		return nil, fmt.Errorf("temporal namespace %q is not configured", namespace)
	}
	return pool, nil
}

// client returns the next client from a namespace's round-robin pool.
func (pool *clientPool) client() workflowservice.WorkflowServiceClient {
	// Round-robin selection spreads concurrent RPCs across the namespace pool.
	return pool.clients[(pool.next.Add(1)-1)%uint64(len(pool.clients))]
}

// StreamWorkflowMetadata streams workflow metadata.
func (s *Source) StreamWorkflowMetadata(
	ctx context.Context,
	req ports.StreamWorkflowMetadataRequest,
) iter.Seq2[*models.WorkflowMetadata, error] {
	return func(yield func(*models.WorkflowMetadata, error) bool) {
		pool, err := s.clientPool(req.Namespace)
		if err != nil {
			yield(nil, err)
			return
		}
		client := pool.client()
		filter := &filterpb.StartTimeFilter{EarliestTime: timestamppb.New(time.Now().Add(-req.Lookback))}
		// Temporal pagination tokens are opaque and must be passed through unchanged.
		var token []byte
		for {
			if limiterErr := pool.metadataLimiter.Wait(ctx); limiterErr != nil {
				yield(nil, limiterErr)
				return
			}
			infos, nextToken, pageErr := listWorkflowMetadataPage(ctx, client, req, filter, token)
			if pageErr != nil {
				yield(nil, pageErr)
				return
			}
			for _, info := range infos {
				if !yield(metadataFromInfo(req.Namespace, info), nil) {
					return
				}
			}
			token = nextToken
			if len(token) == 0 {
				return
			}
		}
	}
}

// listWorkflowMetadataPage fetches one open or closed workflow listing page.
func listWorkflowMetadataPage(
	ctx context.Context,
	client workflowservice.WorkflowServiceClient,
	req ports.StreamWorkflowMetadataRequest,
	filter *filterpb.StartTimeFilter,
	token []byte,
) ([]*workflowpb.WorkflowExecutionInfo, []byte, error) {
	switch req.Type {
	case ports.ListWorkflowsTypeOpen:
		response, err := client.ListOpenWorkflowExecutions(
			ctx,
			&workflowservice.ListOpenWorkflowExecutionsRequest{
				Namespace:       req.Namespace,
				MaximumPageSize: metadataPageSize,
				NextPageToken:   token,
				StartTimeFilter: filter,
			},
		)
		if err != nil {
			return nil, nil, err
		}
		return response.GetExecutions(), response.GetNextPageToken(), nil
	case ports.ListWorkflowsTypeClosed:
		response, err := client.ListClosedWorkflowExecutions(
			ctx,
			&workflowservice.ListClosedWorkflowExecutionsRequest{
				Namespace:       req.Namespace,
				MaximumPageSize: metadataPageSize,
				NextPageToken:   token,
				StartTimeFilter: filter,
			},
		)
		if err != nil {
			return nil, nil, err
		}
		return response.GetExecutions(), response.GetNextPageToken(), nil
	default:
		return nil, nil, fmt.Errorf("unsupported workflow list type %q", req.Type)
	}
}

// FetchWorkflowData retrieves and builds one workflow record from its history and current state.
func (s *Source) FetchWorkflowData(
	ctx context.Context,
	req ports.FetchWorkflowDataRequest,
	mapper ports.Mapper,
) (*models.WorkflowData, error) {
	if req.Metadata == nil {
		return nil, errors.New("workflow metadata is required")
	}
	pool, err := s.clientPool(req.Metadata.Namespace)
	if err != nil {
		return nil, err
	}
	return streamWorkflowData(ctx, pool.client(), *req.Metadata, mapper, pool.dataLimiter)
}

// workflowHistoryPage obtains one complete decoded history page under the namespace data limit.
func workflowHistoryPage(
	ctx context.Context,
	client workflowservice.WorkflowServiceClient,
	metadata models.WorkflowMetadata,
	token []byte,
	limiter *rate.Limiter,
) (*workflowservice.GetWorkflowExecutionHistoryResponse, error) {
	if err := limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return client.GetWorkflowExecutionHistory(ctx, &workflowservice.GetWorkflowExecutionHistoryRequest{
		Namespace:       metadata.Namespace,
		Execution:       &commonpb.WorkflowExecution{WorkflowId: metadata.WorkflowID, RunId: metadata.RunID},
		MaximumPageSize: workflowDataPageSize,
		NextPageToken:   token,
	})
}

type workflowDataBuilder struct {
	data      *models.WorkflowData
	namespace string

	// Temporal lifecycle events refer back to scheduling/initiating event IDs.
	// These indexes let later events update the models created by earlier events.
	activityByID    map[string]int
	activityByEvent map[int64]int
	childByEvent    map[int64]int
}

// newWorkflowDataBuilder creates the aggregate and indexes used while reading history.
func newWorkflowDataBuilder(namespace string) *workflowDataBuilder {
	return &workflowDataBuilder{
		data: &models.WorkflowData{
			Inputs:  make(map[string][]any),
			Outputs: make(map[string][]any),
		},
		namespace:       namespace,
		activityByID:    make(map[string]int),
		activityByEvent: make(map[int64]int),
		childByEvent:    make(map[int64]int),
	}
}

// streamWorkflowData builds one workflow record from its history and current pending state.
func streamWorkflowData(
	ctx context.Context,
	client workflowservice.WorkflowServiceClient,
	metadata models.WorkflowMetadata,
	mapper ports.Mapper,
	limiter *rate.Limiter,
) (*models.WorkflowData, error) {
	// History gives the complete lifecycle; Describe supplies a current snapshot of pending work.
	builder := newWorkflowDataBuilder(metadata.Namespace)
	if err := builder.loadHistory(ctx, client, metadata, mapper, limiter); err != nil {
		return nil, err
	}
	if err := limiter.Wait(ctx); err != nil {
		return nil, err
	}
	description, err := client.DescribeWorkflowExecution(
		ctx,
		&workflowservice.DescribeWorkflowExecutionRequest{
			Namespace: metadata.Namespace,
			Execution: &commonpb.WorkflowExecution{WorkflowId: metadata.WorkflowID, RunId: metadata.RunID},
		},
	)
	if err != nil {
		return nil, err
	}
	builder.mergePending(description)
	return builder.data, nil
}

// loadHistory retrieves and incorporates every page of a workflow's event history.
func (b *workflowDataBuilder) loadHistory(
	ctx context.Context,
	client workflowservice.WorkflowServiceClient,
	metadata models.WorkflowMetadata,
	mapper ports.Mapper,
	limiter *rate.Limiter,
) error {
	var token []byte
	for {
		// A workflow history can be much larger than one response, so consume every page.
		response, err := workflowHistoryPage(ctx, client, metadata, token, limiter)
		if err != nil {
			return err
		}
		if eventsErr := b.addEvents(response.GetHistory().GetEvents(), mapper); eventsErr != nil {
			return eventsErr
		}
		token = response.GetNextPageToken()
		if len(token) == 0 {
			return nil
		}
	}
}

// addEvents applies a history page in Temporal's chronological event order.
func (b *workflowDataBuilder) addEvents(events []*historypb.HistoryEvent, mapper ports.Mapper) error {
	for _, event := range events {
		if err := b.addEvent(event, mapper); err != nil {
			return fmt.Errorf("extract workflow history event %d: %w", event.GetEventId(), err)
		}
	}
	return nil
}

// addEvent routes one history event to each relevant lifecycle extractor.
func (b *workflowDataBuilder) addEvent(event *historypb.HistoryEvent, mapper ports.Mapper) error {
	if err := b.addWorkflowEvent(event, mapper); err != nil {
		return err
	}
	if err := b.addActivityEvent(event, mapper); err != nil {
		return err
	}
	if err := b.addChildWorkflowEvent(event, mapper); err != nil {
		return err
	}
	return nil
}

// addWorkflowEvent extracts parent workflow inputs, outputs and errors.
func (b *workflowDataBuilder) addWorkflowEvent(event *historypb.HistoryEvent, mapper ports.Mapper) error {
	if attributes := event.GetWorkflowExecutionStartedEventAttributes(); attributes != nil {
		inputs, err := mappedPayloads(attributes.GetInput(), mapper)
		if err != nil {
			return err
		}
		b.data.Inputs = inputs
	}
	if attributes := event.GetWorkflowExecutionCompletedEventAttributes(); attributes != nil {
		outputs, err := mappedPayloads(attributes.GetResult(), mapper)
		if err != nil {
			return err
		}
		b.data.Outputs = outputs
	}
	if attributes := event.GetWorkflowExecutionFailedEventAttributes(); attributes != nil {
		b.data.Errors = append(b.data.Errors, failureMessages(attributes.GetFailure())...)
	}
	if event.GetWorkflowExecutionTimedOutEventAttributes() != nil {
		b.data.Errors = append(b.data.Errors, "workflow execution timed out")
	}
	if event.GetWorkflowExecutionCanceledEventAttributes() != nil {
		b.data.Errors = append(b.data.Errors, "workflow execution canceled")
	}
	if attributes := event.GetWorkflowExecutionTerminatedEventAttributes(); attributes != nil {
		b.data.Errors = append(b.data.Errors, terminationMessage(attributes.GetReason()))
	}
	return nil
}

// addActivityEvent reconciles activity lifecycle events with their scheduled activity.
func (b *workflowDataBuilder) addActivityEvent(event *historypb.HistoryEvent, mapper ports.Mapper) error {
	if attributes := event.GetActivityTaskScheduledEventAttributes(); attributes != nil {
		return b.addScheduledActivity(event.GetEventId(), attributes, mapper)
	}
	if attributes := event.GetActivityTaskStartedEventAttributes(); attributes != nil {
		if activity := b.activity(attributes.GetScheduledEventId()); activity != nil {
			activity.Attempts = attributes.GetAttempt()
			activity.StartTime = timeFromProto(event.GetEventTime())
			activity.EndTime = nil
			// On a retry, Temporal repeats the preceding failure here. Keep it for
			// incomplete histories, but do not duplicate the failed-event message.
			activity.Errors = appendFailureMessages(
				activity.Errors,
				attributes.GetLastFailure(),
			)
		}
	}
	if attributes := event.GetActivityTaskCompletedEventAttributes(); attributes != nil {
		return b.completeActivity(
			attributes.GetScheduledEventId(),
			event.GetEventTime(),
			attributes.GetResult(),
			mapper,
		)
	}
	if attributes := event.GetActivityTaskFailedEventAttributes(); attributes != nil {
		b.failActivity(attributes.GetScheduledEventId(), event.GetEventTime(), failureMessages(attributes.GetFailure()))
	}
	if attributes := event.GetActivityTaskTimedOutEventAttributes(); attributes != nil {
		messages := append(failureMessages(attributes.GetFailure()), "activity timed out")
		b.failActivity(attributes.GetScheduledEventId(), event.GetEventTime(), messages)
	}
	if attributes := event.GetActivityTaskCanceledEventAttributes(); attributes != nil {
		b.failActivity(attributes.GetScheduledEventId(), event.GetEventTime(), []string{"activity canceled"})
	}
	return nil
}

// addScheduledActivity records the input and identity shared by later activity events.
func (b *workflowDataBuilder) addScheduledActivity(
	eventID int64,
	attributes *historypb.ActivityTaskScheduledEventAttributes,
	mapper ports.Mapper,
) error {
	// The scheduled event is the stable join key for all subsequent activity events.
	inputs, err := mappedPayloads(attributes.GetInput(), mapper)
	if err != nil {
		return err
	}
	rawActivityID := attributes.GetActivityId()
	b.data.Activities = append(b.data.Activities, models.Activity{
		ID:     customActivityID(rawActivityID),
		Name:   attributes.GetActivityType().GetName(),
		Inputs: inputs,
	})
	index := len(b.data.Activities) - 1
	b.activityByEvent[eventID] = index
	b.activityByID[rawActivityID] = index
	return nil
}

// customActivityID retains a client-provided ID while omitting Temporal's generated integer IDs.
func customActivityID(value string) string {
	if isInteger(value) {
		return ""
	}
	return value
}

// isInteger reports whether value is an integer-form activity ID generated by Temporal.
func isInteger(value string) bool {
	_, err := strconv.ParseInt(value, 10, 64)
	return err == nil
}

// completeActivity maps an activity result and marks its execution complete.
func (b *workflowDataBuilder) completeActivity(
	scheduledEventID int64,
	eventTime *timestamppb.Timestamp,
	payloads *commonpb.Payloads,
	mapper ports.Mapper,
) error {
	activity := b.activity(scheduledEventID)
	if activity == nil {
		return nil
	}
	outputs, err := mappedPayloads(payloads, mapper)
	if err != nil {
		return err
	}
	activity.Outputs = outputs
	activity.EndTime = timePtr(eventTime)
	return nil
}

// failActivity records an activity outcome that ended in an error.
func (b *workflowDataBuilder) failActivity(
	scheduledEventID int64,
	eventTime *timestamppb.Timestamp,
	messages []string,
) {
	// Failed attempts remain useful even when a later retry eventually succeeds.
	activity := b.activity(scheduledEventID)
	if activity == nil {
		return
	}
	activity.Errors = append(activity.Errors, messages...)
	activity.EndTime = timePtr(eventTime)
}

// activity finds the model associated with a Temporal scheduled event ID.
func (b *workflowDataBuilder) activity(scheduledEventID int64) *models.Activity {
	index, ok := b.activityByEvent[scheduledEventID]
	if !ok {
		return nil
	}
	return &b.data.Activities[index]
}

// addChildWorkflowEvent reconciles child workflow lifecycle events with their initiation.
func (b *workflowDataBuilder) addChildWorkflowEvent(event *historypb.HistoryEvent, mapper ports.Mapper) error {
	// Child events use the initiating event ID in the same way activity events use their
	// scheduling event ID, so each outcome can be reconciled with its original input.
	if attributes := event.GetStartChildWorkflowExecutionInitiatedEventAttributes(); attributes != nil {
		return b.addInitiatedChildWorkflow(event.GetEventId(), attributes, mapper)
	}
	if attributes := event.GetChildWorkflowExecutionStartedEventAttributes(); attributes != nil {
		if child := b.child(attributes.GetInitiatedEventId()); child != nil {
			child.WorkflowID = attributes.GetWorkflowExecution().GetWorkflowId()
			child.Namespace = attributes.GetNamespace()
			child.WorkflowType = attributes.GetWorkflowType().GetName()
			child.StartTime = timeFromProto(event.GetEventTime())
		}
	}
	if attributes := event.GetChildWorkflowExecutionCompletedEventAttributes(); attributes != nil {
		return b.completeChildWorkflow(
			attributes.GetInitiatedEventId(),
			event.GetEventTime(),
			attributes.GetResult(),
			mapper,
		)
	}
	if attributes := event.GetChildWorkflowExecutionFailedEventAttributes(); attributes != nil {
		b.failChildWorkflow(
			attributes.GetInitiatedEventId(),
			event.GetEventTime(),
			failureMessages(attributes.GetFailure()),
		)
	}
	if attributes := event.GetChildWorkflowExecutionCanceledEventAttributes(); attributes != nil {
		b.failChildWorkflow(
			attributes.GetInitiatedEventId(),
			event.GetEventTime(),
			[]string{"child workflow canceled"},
		)
	}
	if attributes := event.GetChildWorkflowExecutionTimedOutEventAttributes(); attributes != nil {
		b.failChildWorkflow(
			attributes.GetInitiatedEventId(),
			event.GetEventTime(),
			[]string{"child workflow timed out"},
		)
	}
	if attributes := event.GetChildWorkflowExecutionTerminatedEventAttributes(); attributes != nil {
		b.failChildWorkflow(
			attributes.GetInitiatedEventId(),
			event.GetEventTime(),
			[]string{"child workflow terminated"},
		)
	}
	if attributes := event.GetStartChildWorkflowExecutionFailedEventAttributes(); attributes != nil {
		b.failChildWorkflow(
			attributes.GetInitiatedEventId(),
			event.GetEventTime(),
			[]string{fmt.Sprintf("start child workflow failed: %s", attributes.GetCause())},
		)
	}
	return nil
}

// addInitiatedChildWorkflow records a child workflow before its execution starts.
func (b *workflowDataBuilder) addInitiatedChildWorkflow(
	eventID int64,
	attributes *historypb.StartChildWorkflowExecutionInitiatedEventAttributes,
	mapper ports.Mapper,
) error {
	inputs, err := mappedPayloads(attributes.GetInput(), mapper)
	if err != nil {
		return err
	}
	namespace := attributes.GetNamespace()
	if namespace == "" {
		// An omitted child namespace means the parent workflow's namespace.
		namespace = b.namespace
	}
	b.data.ChildWorkflows = append(b.data.ChildWorkflows, models.ChildWorkflow{
		WorkflowID:   attributes.GetWorkflowId(),
		Namespace:    namespace,
		WorkflowType: attributes.GetWorkflowType().GetName(),
		Inputs:       inputs,
	})
	b.childByEvent[eventID] = len(b.data.ChildWorkflows) - 1
	return nil
}

// completeChildWorkflow maps a child result and records its completion time.
func (b *workflowDataBuilder) completeChildWorkflow(
	initiatedEventID int64,
	eventTime *timestamppb.Timestamp,
	payloads *commonpb.Payloads,
	mapper ports.Mapper,
) error {
	child := b.child(initiatedEventID)
	if child == nil {
		return nil
	}
	outputs, err := mappedPayloads(payloads, mapper)
	if err != nil {
		return err
	}
	child.Outputs = outputs
	child.EndTime = timePtr(eventTime)
	return nil
}

// failChildWorkflow records a terminal child workflow error.
func (b *workflowDataBuilder) failChildWorkflow(
	initiatedEventID int64,
	eventTime *timestamppb.Timestamp,
	messages []string,
) {
	child := b.child(initiatedEventID)
	if child == nil {
		return
	}
	child.Errors = append(child.Errors, messages...)
	child.EndTime = timePtr(eventTime)
}

// child finds the model associated with a Temporal child initiation event ID.
func (b *workflowDataBuilder) child(initiatedEventID int64) *models.ChildWorkflow {
	index, ok := b.childByEvent[initiatedEventID]
	if !ok {
		return nil
	}
	return &b.data.ChildWorkflows[index]
}

// mergePending adds runtime-only pending details to the history-derived data.
func (b *workflowDataBuilder) mergePending(description *workflowservice.DescribeWorkflowExecutionResponse) {
	// History may end before a currently pending item has all details, so merge the
	// point-in-time Describe response without duplicating items already in history.
	for _, activity := range description.GetPendingActivities() {
		if index, ok := b.activityByID[activity.GetActivityId()]; ok {
			b.data.Activities[index].Paused = activity.GetPaused()
			// LastFailure describes the most recent retry attempt, which can occur
			// after the history page was read.
			b.data.Activities[index].Errors = appendFailureMessages(
				b.data.Activities[index].Errors,
				activity.GetLastFailure(),
			)
			continue
		}
		b.data.Activities = append(b.data.Activities, models.Activity{
			ID:        customActivityID(activity.GetActivityId()),
			Name:      activity.GetActivityType().GetName(),
			Errors:    failureMessages(activity.GetLastFailure()),
			Attempts:  activity.GetAttempt(),
			StartTime: timeFromProto(activity.GetLastStartedTime()),
			Paused:    activity.GetPaused(),
		})
	}
	for _, child := range description.GetPendingChildren() {
		if b.hasChild(child.GetWorkflowId()) {
			continue
		}
		b.data.ChildWorkflows = append(b.data.ChildWorkflows, models.ChildWorkflow{
			WorkflowID:   child.GetWorkflowId(),
			Namespace:    b.namespace,
			WorkflowType: child.GetWorkflowTypeName(),
		})
	}
}

// hasChild reports whether history already produced a model for the workflow ID.
func (b *workflowDataBuilder) hasChild(workflowID string) bool {
	for _, child := range b.data.ChildWorkflows {
		if child.WorkflowID == workflowID {
			return true
		}
	}
	return false
}

// mappedPayloads decodes Temporal payloads and groups values by the mapper's field names.
func mappedPayloads(payloads *commonpb.Payloads, mapper ports.Mapper) (map[string][]any, error) {
	fields := make(map[string][]any)
	if mapper == nil {
		return fields, nil
	}
	schema := mapper.Schema()
	for _, payload := range payloads.GetPayloads() {
		value, err := jsonPayload(payload)
		if err != nil {
			return nil, err
		}
		var mappingErr error
		flattenJSON(value, "$", func(key string, value any) {
			if mappingErr != nil {
				return
			}
			field := mapper.Map(key, value)
			if field.Name != "" {
				if validationErr := validateMappedField(schema, key, field); validationErr != nil {
					mappingErr = validationErr
					return
				}
				// Multiple payloads or paths may intentionally map to one result field.
				fields[field.Name] = append(fields[field.Name], field.Value)
			}
		})
		if mappingErr != nil {
			return nil, mappingErr
		}
	}
	return fields, nil
}

// validateMappedField keeps mapper output aligned with the schema used to create
// the OpenSearch mapping. JSON numbers are float64 after [json.Unmarshal].
func validateMappedField(schema types.Schema, path string, field ports.Field) error {
	fieldSchema, ok := schema[field.Name]
	if !ok {
		return fmt.Errorf("mapper returned undeclared field %q for payload path %q", field.Name, path)
	}
	if mappedValueMatchesType(field.Value, fieldSchema.Type) {
		return nil
	}
	return fmt.Errorf(
		"mapper returned %T for field %q at payload path %q; schema requires %s",
		field.Value,
		field.Name,
		path,
		fieldSchema.Type,
	)
}

func mappedValueMatchesType(value any, fieldType types.FieldType) bool {
	switch fieldType {
	case types.FieldTypeKeyword, types.FieldTypeText:
		_, ok := value.(string)
		return ok
	case types.FieldTypeInt:
		number, ok := value.(float64)
		return ok && !math.IsInf(number, 0) && !math.IsNaN(number) && math.Trunc(number) == number
	case types.FieldTypeDouble:
		number, ok := value.(float64)
		return ok && !math.IsInf(number, 0) && !math.IsNaN(number)
	case types.FieldTypeBool:
		_, ok := value.(bool)
		return ok
	case types.FieldTypeTimestamp:
		switch value := value.(type) {
		case time.Time:
			return !value.IsZero()
		case string:
			_, err := time.Parse(time.RFC3339, value)
			return err == nil
		default:
			return false
		}
	default:
		return false
	}
}

// jsonPayload decodes one JSON-encoded Temporal payload into a generic Go value.
func jsonPayload(payload *commonpb.Payload) (any, error) {
	// Payload bytes are decoded before mapping so mappers operate on values, not JSON text.
	var value any
	if err := json.Unmarshal(payload.GetData(), &value); err != nil {
		return nil, fmt.Errorf("decode Temporal payload with encoding %q: %w", payload.GetMetadata()["encoding"], err)
	}
	return value, nil
}

// flattenJSON visits every scalar in a JSON value using dot-separated path keys.
func flattenJSON(value any, key string, visit func(string, any)) {
	switch value := value.(type) {
	case map[string]any:
		// Stable traversal makes repeated extraction deterministic for mappers and callers.
		keys := make([]string, 0, len(value))
		for childKey := range value {
			keys = append(keys, childKey)
		}
		sort.Strings(keys)
		for _, childKey := range keys {
			flattenJSON(value[childKey], key+"."+childKey, visit)
		}
	case []any:
		for index, child := range value {
			flattenJSON(child, fmt.Sprintf("%s.%d", key, index), visit)
		}
	case string:
		// Some SDKs serialize structured values twice. Decode those strings once more so
		// their nested fields remain addressable by normal mapping paths.
		var encoded any
		if err := json.Unmarshal([]byte(value), &encoded); err == nil {
			flattenJSON(encoded, key, visit)
			return
		}
		visit(key, value)
	default:
		visit(key, value)
	}
}

// failureMessages returns the messages from a Temporal failure and all its causes.
func failureMessages(failure *failurepb.Failure) []string {
	// Temporal failures form a causal chain; retain each non-empty message for diagnostics.
	var messages []string
	for failure != nil {
		if message := failure.GetMessage(); message != "" {
			messages = append(messages, message)
		}
		failure = failure.GetCause()
	}
	return messages
}

// appendFailureMessages adds an unseen Temporal failure chain to activity errors.
func appendFailureMessages(errors []string, failure *failurepb.Failure) []string {
	for _, message := range failureMessages(failure) {
		seen := slices.Contains(errors, message)
		if !seen {
			errors = append(errors, message)
		}
	}
	return errors
}

// terminationMessage returns a useful default when Temporal provides no termination reason.
func terminationMessage(reason string) string {
	if reason == "" {
		return "workflow execution terminated"
	}
	return reason
}

// timePtr converts an optional protobuf timestamp to an optional Go timestamp.
func timePtr(timestamp *timestamppb.Timestamp) *time.Time {
	value := timeFromProto(timestamp)
	if value.IsZero() {
		return nil
	}
	return &value
}

// metadataFromInfo adapts Temporal's list response into the domain metadata model.
func metadataFromInfo(namespace string, info *workflowpb.WorkflowExecutionInfo) *models.WorkflowMetadata {
	end := timeFromProto(info.GetCloseTime())
	var endPtr *time.Time
	if !end.IsZero() {
		endPtr = &end
	}
	attrs := make([]models.SearchAttribute, 0, len(info.GetSearchAttributes().GetIndexedFields()))
	// Search attribute values are opaque payload bytes at this metadata layer.
	for key, value := range info.GetSearchAttributes().GetIndexedFields() {
		attrs = append(attrs, models.SearchAttribute{Key: key, Value: string(value.GetData())})
	}
	return &models.WorkflowMetadata{
		Namespace:            namespace,
		WorkflowID:           info.GetExecution().GetWorkflowId(),
		RunID:                info.GetExecution().GetRunId(),
		WorkflowType:         info.GetType().GetName(),
		StartTime:            timeFromProto(info.GetStartTime()),
		EndTime:              endPtr,
		Status:               statusFromTemporal(info.GetStatus()),
		StateTransitionCount: info.GetStateTransitionCount(),
		SearchAttributes:     attrs,
	}
}

// timeFromProto converts an absent protobuf timestamp to Go's zero time.
func timeFromProto(timestamp *timestamppb.Timestamp) time.Time {
	// Protobuf getters legitimately return nil for absent optional timestamps.
	if timestamp == nil {
		return time.Time{}
	}
	return timestamp.AsTime()
}

// statusFromTemporal maps Temporal statuses to the domain-level status vocabulary.
func statusFromTemporal(status enumspb.WorkflowExecutionStatus) models.Status {
	// Keep Temporal-specific statuses at the adapter boundary.
	switch status {
	case enumspb.WORKFLOW_EXECUTION_STATUS_PAUSED:
		return models.StatusPaused
	case enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return models.StatusRunning
	case enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return models.StatusCompleted
	case enumspb.WORKFLOW_EXECUTION_STATUS_FAILED:
		return models.StatusFailed
	case enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return models.StatusTimedOut
	case enumspb.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
		return models.StatusContinuedAsNew
	case enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return models.StatusCanceled
	case enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return models.StatusTerminated
	case enumspb.WORKFLOW_EXECUTION_STATUS_UNSPECIFIED:
		return models.StatusUnknown
	default:
		return models.StatusUnknown
	}
}

// Signal signals Temporal workflows.
func (s *Source) Signal(ctx context.Context, req ports.InternalSignalRequest) error {
	// Batch operations are namespace-scoped and asynchronous: success means Temporal accepted each job.
	return s.forEachNamespace(
		ctx,
		req.Executions,
		func(ctx context.Context, pool *clientPool, namespace string, executions []ports.ExecutionInfo) error {
			_, err := pool.client().StartBatchOperation(
				ctx,
				&workflowservice.StartBatchOperationRequest{
					Namespace:              namespace,
					TargetExecutions:       targetExecutions(executions),
					JobId:                  uuid.NewString(),
					Reason:                 req.Reason,
					MaxOperationsPerSecond: pool.bulkActionsPerSecond,
					Operation: &workflowservice.StartBatchOperationRequest_SignalOperation{
						SignalOperation: &batchpb.BatchOperationSignal{
							Signal: req.Signal,
							Input: &commonpb.Payloads{Payloads: []*commonpb.Payload{
								// The request payload contains JSON bytes, which Temporal SDKs decode with json/plain.
								{Metadata: map[string][]byte{"encoding": []byte("json/plain")}, Data: req.Payload},
							}},
							Identity: clientIdentity,
						},
					},
				},
			)
			return err
		},
	)
}

// Reset resets Temporal workflows using a native batch operation.
func (s *Source) Reset(ctx context.Context, req ports.InternalResetRequest) error {
	// Batch operations are namespace-scoped and asynchronous: success means Temporal accepted each job.
	options, optionsErr := batchResetOptions(req.Target)
	if optionsErr != nil {
		return optionsErr
	}
	return s.forEachNamespace(
		ctx,
		req.Executions,
		func(ctx context.Context, pool *clientPool, namespace string, executions []ports.ExecutionInfo) error {
			_, err := pool.client().StartBatchOperation(
				ctx,
				&workflowservice.StartBatchOperationRequest{
					Namespace:              namespace,
					TargetExecutions:       targetExecutions(executions),
					JobId:                  uuid.NewString(),
					Reason:                 req.Reason,
					MaxOperationsPerSecond: pool.bulkActionsPerSecond,
					Operation: &workflowservice.StartBatchOperationRequest_ResetOperation{
						ResetOperation: &batchpb.BatchOperationReset{
							Identity: clientIdentity,
							Options:  options,
						},
					},
				},
			)
			return err
		},
	)
}

func batchResetOptions(target ports.ResetTarget) (*commonpb.ResetOptions, error) {
	switch target.Kind {
	case ports.ResetTargetFirstWorkflowTask:
		return &commonpb.ResetOptions{Target: &commonpb.ResetOptions_FirstWorkflowTask{
			FirstWorkflowTask: &emptypb.Empty{},
		}}, nil
	case ports.ResetTargetLastWorkflowTask:
		return &commonpb.ResetOptions{Target: &commonpb.ResetOptions_LastWorkflowTask{
			LastWorkflowTask: &emptypb.Empty{},
		}}, nil
	case ports.ResetTargetWorkflowTaskID:
		if target.WorkflowTaskID < 1 {
			return nil, fmt.Errorf("workflow task ID must be positive")
		}
		return &commonpb.ResetOptions{Target: &commonpb.ResetOptions_WorkflowTaskId{
			WorkflowTaskId: target.WorkflowTaskID,
		}}, nil
	default:
		return nil, fmt.Errorf("reset target is required")
	}
}

// Cancel requests cancellation of Temporal workflows.
func (s *Source) Cancel(ctx context.Context, req ports.InternalCancelRequest) error {
	// Batch operations are namespace-scoped and asynchronous: success means Temporal accepted each job.
	return s.forEachNamespace(
		ctx,
		req.Executions,
		func(ctx context.Context, pool *clientPool, namespace string, executions []ports.ExecutionInfo) error {
			_, err := pool.client().StartBatchOperation(
				ctx,
				&workflowservice.StartBatchOperationRequest{
					Namespace:              namespace,
					TargetExecutions:       targetExecutions(executions),
					JobId:                  uuid.NewString(),
					Reason:                 req.Reason,
					MaxOperationsPerSecond: pool.bulkActionsPerSecond,
					Operation: &workflowservice.StartBatchOperationRequest_CancellationOperation{
						CancellationOperation: &batchpb.BatchOperationCancellation{Identity: clientIdentity},
					},
				},
			)
			return err
		},
	)
}

// Terminate terminates Temporal workflows.
func (s *Source) Terminate(ctx context.Context, req ports.InternalTerminateRequest) error {
	// Batch operations are namespace-scoped and asynchronous: success means Temporal accepted each job.
	return s.forEachNamespace(
		ctx,
		req.Executions,
		func(ctx context.Context, pool *clientPool, namespace string, executions []ports.ExecutionInfo) error {
			_, err := pool.client().StartBatchOperation(
				ctx,
				&workflowservice.StartBatchOperationRequest{
					Namespace:              namespace,
					TargetExecutions:       targetExecutions(executions),
					JobId:                  uuid.NewString(),
					Reason:                 req.Reason,
					MaxOperationsPerSecond: pool.bulkActionsPerSecond,
					Operation: &workflowservice.StartBatchOperationRequest_TerminationOperation{
						TerminationOperation: &batchpb.BatchOperationTermination{
							Identity: clientIdentity,
						},
					},
				},
			)
			return err
		},
	)
}

// forEachNamespace processes each namespace concurrently while preserving ordering within it.
func (s *Source) forEachNamespace(
	ctx context.Context,
	executions []ports.ExecutionInfo,
	action func(context.Context, *clientPool, string, []ports.ExecutionInfo) error,
) error {
	groups := executionsByNamespace(executions)
	group, groupCtx := errgroup.WithContext(ctx)
	for namespace, namespaceExecutions := range groups {
		pool, err := s.clientPool(namespace)
		if err != nil {
			return err
		}
		group.Go(func() error {
			return action(groupCtx, pool, namespace, namespaceExecutions)
		})
	}
	return group.Wait()
}

// executionsByNamespace groups concrete workflow targets for namespace-scoped Temporal APIs.
func executionsByNamespace(executions []ports.ExecutionInfo) map[string][]ports.ExecutionInfo {
	groups := make(map[string][]ports.ExecutionInfo)
	for _, execution := range executions {
		groups[execution.Namespace] = append(groups[execution.Namespace], execution)
	}
	return groups
}

// targetExecutions converts domain execution identities to Temporal's batch-operation targets.
func targetExecutions(executions []ports.ExecutionInfo) []*commonpb.Execution {
	targets := make([]*commonpb.Execution, 0, len(executions))
	for _, execution := range executions {
		targets = append(targets, &commonpb.Execution{BusinessId: execution.WorkflowID, RunId: execution.RunID})
	}
	return targets
}

// WorkflowURL returns Temporal UI's canonical workflow route for a workflow execution.
func (s *Source) WorkflowURL(_ context.Context, metadata models.WorkflowMetadata) (string, error) {
	if metadata.Namespace == "" || metadata.WorkflowID == "" || metadata.RunID == "" {
		return "", errors.New("namespace, workflow ID, and run ID are required")
	}
	target := *s.baseURL
	// Path is the decoded representation used by net/url. RawPath preserves escaped
	// identifiers (especially embedded '/') as a single Temporal UI route segment.
	target.Path = path.Join(
		target.Path,
		"namespaces",
		metadata.Namespace,
		"workflows",
		metadata.WorkflowID,
		metadata.RunID,
	)
	target.RawPath = path.Join(
		s.baseURL.EscapedPath(),
		"namespaces",
		url.PathEscape(metadata.Namespace),
		"workflows",
		url.PathEscape(metadata.WorkflowID),
		url.PathEscape(metadata.RunID),
	)
	return target.String(), nil
}
