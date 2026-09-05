package workflows_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"connectrpc.com/connect"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	grpcutil "github.com/varunbpatil/temporal-lens/inbound/grpc"
	"github.com/varunbpatil/temporal-lens/inbound/grpc/workflows"
	v1 "github.com/varunbpatil/temporal-lens/protos/gen/temporal_lens/workflows/v1"
	"github.com/varunbpatil/temporal-lens/protos/gen/temporal_lens/workflows/v1/workflowsv1connect"
)

type brotliTestService struct {
	workflowsv1connect.UnimplementedWorkflowServiceHandler
}

func (s *brotliTestService) Search(
	context.Context,
	*connect.Request[v1.SearchRequest],
) (*connect.Response[v1.SearchResponse], error) {
	return connect.NewResponse(&v1.SearchResponse{}), nil
}

type captureTransport struct {
	handler     http.Handler
	mu          sync.Mutex
	requestHdr  http.Header
	responseHdr http.Header
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	if err = req.Body.Close(); err != nil {
		return nil, err
	}

	t.mu.Lock()
	t.requestHdr = req.Header.Clone()
	t.mu.Unlock()

	inbound := req.Clone(req.Context())
	inbound.Body = io.NopCloser(bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, inbound)
	res := recorder.Result()

	t.mu.Lock()
	t.responseHdr = res.Header.Clone()
	t.mu.Unlock()

	return res, nil
}

func TestRegisterUsesBrotliCompression(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	workflows.Register(mux, &brotliTestService{})

	capTransport := &captureTransport{handler: mux}
	client := &http.Client{Transport: capTransport}

	c := workflowsv1connect.NewWorkflowServiceClient(
		client,
		"http://example.com",
		connect.WithSendCompression(grpcutil.Brotli),
		connect.WithAcceptCompression(grpcutil.Brotli, grpcutil.NewBrotliDecompressor, grpcutil.NewBrotliCompressor),
	)

	_, err := c.Search(context.Background(), connect.NewRequest(&v1.SearchRequest{}))
	require.NoError(t, err)

	capTransport.mu.Lock()
	defer capTransport.mu.Unlock()

	assert.Equal(t, grpcutil.Brotli, capTransport.requestHdr.Get("Content-Encoding"))
	assert.Equal(t, grpcutil.Brotli, capTransport.responseHdr.Get("Content-Encoding"))
}

func TestRegisterUsesBrotliCompressionForGRPC(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	workflows.Register(mux, &brotliTestService{})

	capTransport := &captureTransport{handler: mux}
	client := &http.Client{Transport: capTransport}

	c := workflowsv1connect.NewWorkflowServiceClient(
		client,
		"http://example.com",
		connect.WithGRPC(),
		connect.WithSendCompression(grpcutil.Brotli),
		connect.WithAcceptCompression(grpcutil.Brotli, grpcutil.NewBrotliDecompressor, grpcutil.NewBrotliCompressor),
	)

	_, err := c.Search(context.Background(), connect.NewRequest(&v1.SearchRequest{}))
	require.NoError(t, err)

	capTransport.mu.Lock()
	defer capTransport.mu.Unlock()

	assert.Equal(t, grpcutil.Brotli, capTransport.requestHdr.Get("Grpc-Encoding"))
	assert.Equal(t, grpcutil.Brotli, capTransport.responseHdr.Get("Grpc-Encoding"))
}
