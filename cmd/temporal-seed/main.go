// Command temporal-seed creates deterministic, intentionally varied workflow histories.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	operatorservice "go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/serviceerror"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	taskQueue                 = "temporal-lens"
	activityTimeout           = time.Second
	workflowTimeout           = 2 * time.Minute
	timedOutWorkflowLimit     = time.Second
	retryInitialInterval      = 100 * time.Millisecond
	visibilityRetryDelay      = time.Second
	visibilityRetryCount      = 30
	namespaceRetention        = 24 * time.Hour
	activityScheduleTimeout   = 5 * time.Second
	timeoutSleepMultiplier    = 2
	retryMaximumAttempts      = 3
	outcomeModulo             = 3
	failureActivityModulo     = 2
	childAddressWorkflow      = "ChildAddressWorkflow"
	childNotificationWorkflow = "ChildNotificationWorkflow"
	childAuditWorkflow        = "ChildAuditWorkflow"
	childInventoryWorkflow    = "ChildInventoryWorkflow"
)

//nolint:gochecknoglobals // fixed seed catalog
var namespaces = []string{
	"orders",
	"payments",
	"fulfillment",
	"support",
}

//nolint:gochecknoglobals // fixed seed catalog
var searchAttributes = map[string]enumspb.IndexedValueType{
	"SeedWorkflowType": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
	"SeedScenario":     enumspb.INDEXED_VALUE_TYPE_KEYWORD,
	"SeedIndex":        enumspb.INDEXED_VALUE_TYPE_INT,
	"SeedHasFailure":   enumspb.INDEXED_VALUE_TYPE_BOOL,
}

type seedConfig struct {
	endpoint                       string
	count, maxConcurrentActivities int
	seed                           int64
}

type workflowDefinition struct {
	Name                 string
	Activities, Children []string
}

//nolint:gochecknoglobals // fixed seed catalog
var definitions = []workflowDefinition{
	{
		Name:       "SeedOrderWorkflow",
		Activities: []string{"ValidateOrder", "ReserveInventory", "AuthorizePayment", "PackOrder", "ShipOrder"},
		Children:   []string{childAddressWorkflow, childNotificationWorkflow},
	},
	{
		Name:       "SeedPaymentWorkflow",
		Activities: []string{"LoadPayment", "FraudCheck", "CapturePayment", "WriteLedger", "ReceiptPayment"},
		Children:   []string{childNotificationWorkflow, childAuditWorkflow},
	},
	{
		Name:       "SeedFulfillmentWorkflow",
		Activities: []string{"LoadCart", "PickItems", "PackItems", "CreateLabel", "DispatchItems"},
		Children:   []string{childInventoryWorkflow, childNotificationWorkflow},
	},
	{
		Name:       "SeedSupportWorkflow",
		Activities: []string{"LoadTicket", "ClassifyTicket", "FindArticle", "AssignAgent", "CloseTicket"},
		Children:   []string{childAuditWorkflow, childNotificationWorkflow},
	},
}

//nolint:gochecknoglobals // fixed seed catalog
var childDefinitions = map[string]workflowDefinition{
	childAddressWorkflow: {
		Name:       childAddressWorkflow,
		Activities: []string{"ReadAddress", "NormalizeAddress", "GeocodeAddress", "SaveAddress"},
	},
	childNotificationWorkflow: {
		Name:       childNotificationWorkflow,
		Activities: []string{"BuildMessage", "RenderMessage", "SendMessage", "RecordNotification"},
	},
	childAuditWorkflow: {
		Name:       childAuditWorkflow,
		Activities: []string{"BuildAudit", "EnrichAudit", "PersistAudit", "IndexAudit"},
	},
	childInventoryWorkflow: {
		Name:       childInventoryWorkflow,
		Activities: []string{"ReadStock", "ReserveStock", "UpdateStock", "PublishStock"},
	},
}

type outcome string

const (
	outcomeSuccess outcome = "success"
	outcomeFailure outcome = "failed"
	outcomeTimeout outcome = "timed_out"
)

type workflowInput struct {
	Index      int64
	Definition string
	Outcome    outcome
}

type workflowOutput struct {
	Index      int64
	Definition string
	Activities []activityOutput
	Children   []workflowOutput
}

type activityInput struct {
	Index    int64
	Name     string
	Outcome  outcome
	Sequence int
}

type activityOutput struct {
	Name     string
	Sequence int
	Value    string
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "temporal-seed:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg := seedConfig{}
	flag.StringVar(&cfg.endpoint, "endpoint", "localhost:7233", "Temporal gRPC endpoint")
	flag.IntVar(&cfg.count, "count", 100, "number of workflows to start")
	flag.IntVar(&cfg.maxConcurrentActivities, "workers", 100, "concurrent activities per namespace")
	flag.Int64Var(&cfg.seed, "seed", 1, "stable seed used to select scenarios")
	flag.Parse()
	if cfg.count < 1 {
		return errors.New("count must be positive")
	}
	if cfg.maxConcurrentActivities < 1 {
		return errors.New("workers must be positive")
	}
	bootstrap, err := client.Dial(client.Options{HostPort: cfg.endpoint})
	if err != nil {
		return fmt.Errorf("connect to Temporal: %w", err)
	}
	defer bootstrap.Close()
	if namespaceErr := ensureNamespaces(ctx, bootstrap); namespaceErr != nil {
		return namespaceErr
	}
	clients := make(map[string]client.Client, len(namespaces))
	workers := make([]worker.Worker, 0, len(namespaces))
	defer func() {
		for _, w := range workers {
			w.Stop()
		}
		for _, c := range clients {
			c.Close()
		}
	}()
	for _, namespace := range namespaces {
		namespaceClient, dialErr := client.Dial(client.Options{HostPort: cfg.endpoint, Namespace: namespace})
		if dialErr != nil {
			return fmt.Errorf("connect to namespace %q: %w", namespace, dialErr)
		}
		clients[namespace] = namespaceClient
		w := newWorker(namespaceClient, cfg.maxConcurrentActivities)
		if startErr := w.Start(); startErr != nil {
			return fmt.Errorf("start worker for namespace %q: %w", namespace, startErr)
		}
		workers = append(workers, w)
	}
	// Search-attribute propagation is eventually consistent. IDs are reject-duplicate,
	// so repeating this complete set can only obtain the original executions.
	return retry(
		ctx,
		visibilityRetryCount,
		visibilityRetryDelay,
		func() error { return executeWorkflows(ctx, clients, cfg) },
	)
}

func ensureNamespaces(ctx context.Context, c client.Client) error {
	for _, namespace := range namespaces {
		_, err := c.WorkflowService().
			DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{Namespace: namespace})
		if isNamespaceNotFound(err) {
			_, err = c.WorkflowService().RegisterNamespace(ctx, &workflowservice.RegisterNamespaceRequest{
				Namespace: namespace, Description: "Temporal Lens deterministic seed data",
				WorkflowExecutionRetentionPeriod: durationpb.New(namespaceRetention),
			})
		}
		if err != nil && !isAlreadyExists(err) {
			return fmt.Errorf("ensure namespace %q: %w", namespace, err)
		}
		if namespaceReadyErr := waitForNamespace(ctx, c, namespace); namespaceReadyErr != nil {
			return namespaceReadyErr
		}
		attributesErr := retry(ctx, visibilityRetryCount, visibilityRetryDelay, func() error {
			return ensureSearchAttributes(ctx, c, namespace)
		})
		if attributesErr != nil {
			return attributesErr
		}
	}
	return nil
}

func waitForNamespace(ctx context.Context, c client.Client, namespace string) error {
	err := retry(ctx, visibilityRetryCount, visibilityRetryDelay, func() error {
		_, describeErr := c.WorkflowService().
			DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{Namespace: namespace})
		return describeErr
	})
	if err != nil {
		return fmt.Errorf("wait for namespace %q: %w", namespace, err)
	}
	return nil
}

func ensureSearchAttributes(ctx context.Context, c client.Client, namespace string) error {
	response, err := c.WorkflowService().GetSearchAttributes(ctx, &workflowservice.GetSearchAttributesRequest{})
	if err != nil {
		return fmt.Errorf("get search attributes for namespace %q: %w", namespace, err)
	}
	missing := make(map[string]enumspb.IndexedValueType)
	for name, kind := range searchAttributes {
		if _, found := response.GetKeys()[name]; !found {
			missing[name] = kind
		}
	}
	if len(missing) == 0 {
		return nil
	}
	_, err = c.OperatorService().
		AddSearchAttributes(ctx, &operatorservice.AddSearchAttributesRequest{Namespace: namespace, SearchAttributes: missing})
	if err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("add search attributes for namespace %q: %w", namespace, err)
	}
	return nil
}

func isNamespaceNotFound(err error) bool {
	var namespaceNotFound *serviceerror.NamespaceNotFound
	return errors.As(err, &namespaceNotFound) || status.Code(err) == codes.NotFound
}

func isAlreadyExists(err error) bool {
	var namespaceAlreadyExists *serviceerror.NamespaceAlreadyExists
	return errors.As(err, &namespaceAlreadyExists) || status.Code(err) == codes.AlreadyExists
}

func newWorker(c client.Client, maxActivities int) worker.Worker {
	w := worker.New(
		c,
		taskQueue,
		worker.Options{MaxConcurrentActivityExecutionSize: maxActivities, DisableRegistrationAliasing: true},
	)
	for _, definition := range definitions {
		w.RegisterWorkflowWithOptions(
			seedWorkflow,
			workflow.RegisterOptions{Name: definition.Name, DisableAlreadyRegisteredCheck: true},
		)
	}
	for name := range childDefinitions {
		w.RegisterWorkflowWithOptions(
			seedChildWorkflow,
			workflow.RegisterOptions{Name: name, DisableAlreadyRegisteredCheck: true},
		)
	}
	for _, name := range activityNames() {
		w.RegisterActivityWithOptions(
			runActivity,
			activity.RegisterOptions{Name: name, DisableAlreadyRegisteredCheck: true},
		)
	}
	return w
}

func activityNames() []string {
	seen := make(map[string]struct{})
	for _, definition := range definitions {
		for _, name := range definition.Activities {
			seen[name] = struct{}{}
		}
	}
	for _, definition := range childDefinitions {
		for _, name := range definition.Activities {
			seen[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func executeWorkflows(ctx context.Context, clients map[string]client.Client, cfg seedConfig) error {
	handles := make([]client.WorkflowRun, 0, cfg.count)
	for i := range cfg.count {
		index := cfg.seed + int64(i)
		definition := definitions[i%len(definitions)]
		namespace := namespaces[i%len(namespaces)]
		input := workflowInput{Index: index, Definition: definition.Name, Outcome: outcomeFor(index, 0)}
		options := client.StartWorkflowOptions{
			ID: fmt.Sprintf(
				"seed-%s-%d",
				definition.Name,
				index,
			),
			TaskQueue:                taskQueue,
			WorkflowExecutionTimeout: workflowTimeout,
			WorkflowIDReusePolicy:    enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
			TypedSearchAttributes:    seedSearchAttributes(definition.Name, input),
		}
		if input.Outcome == outcomeTimeout {
			options.WorkflowExecutionTimeout = timedOutWorkflowLimit
		}
		handle, err := clients[namespace].ExecuteWorkflow(ctx, options, definition.Name, input)
		if temporal.IsWorkflowExecutionAlreadyStartedError(err) {
			handle = clients[namespace].GetWorkflow(ctx, options.ID, "")
			err = nil
		}
		if err != nil {
			return fmt.Errorf("start %s/%s: %w", namespace, options.ID, err)
		}
		handles = append(handles, handle)
	}
	var wait sync.WaitGroup
	results := make(chan error, len(handles))
	for _, handle := range handles {
		wait.Add(1)
		go func(h client.WorkflowRun) {
			defer wait.Done()
			var output workflowOutput
			if err := h.Get(ctx, &output); err != nil && !expectedSeedError(err) {
				results <- err
			}
		}(handle)
	}
	wait.Wait()
	close(results)
	for err := range results {
		return fmt.Errorf("wait for seeded workflow: %w", err)
	}
	return nil
}

func seedSearchAttributes(definition string, input workflowInput) temporal.SearchAttributes {
	workflowType := temporal.NewSearchAttributeKeyKeyword("SeedWorkflowType")
	scenario := temporal.NewSearchAttributeKeyKeyword("SeedScenario")
	index := temporal.NewSearchAttributeKeyInt64("SeedIndex")
	hasFailure := temporal.NewSearchAttributeKeyBool("SeedHasFailure")
	return temporal.NewSearchAttributes(
		workflowType.ValueSet(definition),
		scenario.ValueSet(string(input.Outcome)),
		index.ValueSet(input.Index),
		hasFailure.ValueSet(input.Outcome != outcomeSuccess),
	)
}

func expectedSeedError(err error) bool {
	var applicationErr *temporal.ApplicationError
	var timeoutErr *temporal.TimeoutError
	return errors.As(err, &applicationErr) || errors.As(err, &timeoutErr)
}

func seedWorkflow(ctx workflow.Context, input workflowInput) (workflowOutput, error) {
	if input.Outcome == outcomeTimeout {
		return workflowOutput{}, workflow.Sleep(ctx, timeoutSleepMultiplier*timedOutWorkflowLimit)
	}
	return runDefinition(ctx, definitionFor(input.Definition, definitions), input, true)
}

func seedChildWorkflow(ctx workflow.Context, input workflowInput) (workflowOutput, error) {
	if input.Outcome == outcomeTimeout {
		return workflowOutput{}, workflow.Sleep(ctx, timeoutSleepMultiplier*timedOutWorkflowLimit)
	}
	definition, found := childDefinitions[input.Definition]
	if !found {
		return workflowOutput{}, temporal.NewNonRetryableApplicationError(
			"unknown child definition",
			"SeedDefinitionError",
			nil,
		)
	}
	return runDefinition(ctx, definition, input, false)
}

func runDefinition(
	ctx workflow.Context,
	definition workflowDefinition,
	input workflowInput,
	parent bool,
) (workflowOutput, error) {
	output := workflowOutput{Index: input.Index, Definition: definition.Name}
	failLastActivity := input.Outcome == outcomeFailure && (!parent || failureByActivity(input.Index))
	activityOutput, err := runActivities(ctx, definition.Activities, input, failLastActivity)
	output.Activities = activityOutput
	if err != nil {
		return output, err
	}
	if !parent {
		return output, nil
	}
	children, err := runChildren(ctx, definition, input, failLastActivity)
	output.Children = children
	return output, err
}

func failureByActivity(index int64) bool {
	// Parent indexes advance by len(namespaces) within a namespace. Dividing by
	// that stride makes failure placement alternate for each namespace instead
	// of correlating it with the namespace's position in the catalog.
	return (index/int64(len(namespaces)))%failureActivityModulo != 0
}

func runActivities(
	ctx workflow.Context,
	activities []string,
	input workflowInput,
	failLastActivity bool,
) ([]activityOutput, error) {
	output := make([]activityOutput, 0, len(activities))
	for position, name := range activities {
		result, err := executeActivity(ctx, activityInput{
			Index:    input.Index,
			Name:     name,
			Outcome:  activityOutcome(input.Index, position, len(activities), failLastActivity),
			Sequence: position,
		})
		if err != nil {
			if position == len(activities)-1 {
				return output, err
			}
			continue // Intermediate activity errors intentionally continue.
		}
		output = append(output, result)
	}
	return output, nil
}

func activityOutcome(index int64, position, count int, failLast bool) outcome {
	if position != count-1 {
		return outcomeFor(index, position+1)
	}
	if failLast {
		return outcomeFailure
	}
	return outcomeSuccess
}

func runChildren(
	ctx workflow.Context,
	definition workflowDefinition,
	input workflowInput,
	failedByActivity bool,
) ([]workflowOutput, error) {
	output := make([]workflowOutput, 0, len(definition.Children))
	for position, name := range definition.Children {
		childOutput, err := executeSeedChild(
			ctx,
			name,
			input,
			position,
			len(definition.Children),
			len(definition.Activities),
			failedByActivity,
		)
		if err != nil {
			if position == len(definition.Children)-1 {
				return output, err
			}
			continue // Intermediate child errors intentionally continue.
		}
		output = append(output, childOutput)
	}
	return output, nil
}

func executeSeedChild(
	ctx workflow.Context,
	name string,
	input workflowInput,
	position, childCount, activityCount int,
	failedByActivity bool,
) (workflowOutput, error) {
	childOutcome := outcomeFor(input.Index, activityCount+position+1)
	if position == childCount-1 {
		if input.Outcome == outcomeFailure && !failedByActivity {
			childOutcome = outcomeFailure
		} else {
			childOutcome = outcomeSuccess
		}
	}
	childTimeout := workflowTimeout
	if childOutcome == outcomeTimeout {
		childTimeout = timedOutWorkflowLimit
	}
	childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
		WorkflowID: fmt.Sprintf("seed-child-%s-%d-%d", name, input.Index, position), TaskQueue: taskQueue,
		WorkflowExecutionTimeout: childTimeout, RetryPolicy: retryPolicy(),
	})
	var output workflowOutput
	err := workflow.ExecuteChildWorkflow(childCtx, name, workflowInput{Index: input.Index, Definition: name, Outcome: childOutcome}).
		Get(childCtx, &output)
	return output, err
}

func executeActivity(ctx workflow.Context, input activityInput) (activityOutput, error) {
	activityCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		TaskQueue:              taskQueue,
		StartToCloseTimeout:    activityTimeout,
		ScheduleToCloseTimeout: activityScheduleTimeout,
		RetryPolicy:            retryPolicy(),
	})
	var output activityOutput
	err := workflow.ExecuteActivity(activityCtx, input.Name, input).Get(activityCtx, &output)
	return output, err
}

func runActivity(ctx context.Context, input activityInput) (activityOutput, error) {
	if input.Outcome == outcomeTimeout {
		select {
		case <-time.After(timeoutSleepMultiplier * activityTimeout):
			return activityOutput{}, nil
		case <-ctx.Done():
			return activityOutput{}, ctx.Err()
		}
	}
	if input.Outcome == outcomeFailure {
		return activityOutput{}, temporal.NewApplicationError(
			fmt.Sprintf("%s failed", input.Name),
			input.Name+"Failure",
		)
	}
	return activityOutput{
		Name:     input.Name,
		Sequence: input.Sequence,
		Value:    fmt.Sprintf("%s-%d", input.Name, input.Index),
	}, nil
}

func retryPolicy() *temporal.RetryPolicy {
	return &temporal.RetryPolicy{
		InitialInterval:    retryInitialInterval,
		BackoffCoefficient: 1,
		MaximumAttempts:    retryMaximumAttempts,
	}
}

func outcomeFor(index int64, position int) outcome {
	switch (index + int64(position)) % outcomeModulo {
	case 0:
		return outcomeSuccess
	case 1:
		return outcomeFailure
	default:
		return outcomeTimeout
	}
}

func definitionFor(name string, catalog []workflowDefinition) workflowDefinition {
	for _, definition := range catalog {
		if definition.Name == name {
			return definition
		}
	}
	return workflowDefinition{}
}

func retry(ctx context.Context, attempts int, delay time.Duration, operation func() error) error {
	var err error
	for attempt := range attempts {
		if err = operation(); err == nil {
			return nil
		}
		if attempt == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}
