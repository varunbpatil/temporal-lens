package grpc_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	grpcutil "github.com/varunbpatil/temporal-lens/inbound/grpc"
)

const compressionTestProcedure = "/temporal_lens.test.v1.CompressionService/Compress"

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

func newCompressionTestHandler() http.Handler {
	mux := http.NewServeMux()
	handler := connect.NewUnaryHandler(
		compressionTestProcedure,
		func(
			context.Context,
			*connect.Request[emptypb.Empty],
		) (*connect.Response[emptypb.Empty], error) {
			return connect.NewResponse(&emptypb.Empty{}), nil
		},
		connect.WithHandlerOptions(grpcutil.HandlerOptions()...),
	)
	mux.Handle(compressionTestProcedure, handler)
	return mux
}

func TestHandlerOptionsUseBrotliCompressionForHTTP(t *testing.T) {
	t.Parallel()
	capTransport := &captureTransport{handler: newCompressionTestHandler()}
	client := &http.Client{Transport: capTransport}

	c := connect.NewClient[emptypb.Empty, emptypb.Empty](
		client,
		"http://example.com"+compressionTestProcedure,
		connect.WithSendCompression(grpcutil.Brotli),
		connect.WithAcceptCompression(grpcutil.Brotli, grpcutil.NewBrotliDecompressor, grpcutil.NewBrotliCompressor),
	)

	_, err := c.CallUnary(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	require.NoError(t, err)

	capTransport.mu.Lock()
	defer capTransport.mu.Unlock()

	assert.Equal(t, grpcutil.Brotli, capTransport.requestHdr.Get("Content-Encoding"))
	assert.Equal(t, grpcutil.Brotli, capTransport.responseHdr.Get("Content-Encoding"))
}

func TestHandlerOptionsUseBrotliCompressionForGRPC(t *testing.T) {
	t.Parallel()
	capTransport := &captureTransport{handler: newCompressionTestHandler()}
	client := &http.Client{Transport: capTransport}

	c := connect.NewClient[emptypb.Empty, emptypb.Empty](
		client,
		"http://example.com"+compressionTestProcedure,
		connect.WithGRPC(),
		connect.WithSendCompression(grpcutil.Brotli),
		connect.WithAcceptCompression(grpcutil.Brotli, grpcutil.NewBrotliDecompressor, grpcutil.NewBrotliCompressor),
	)

	_, err := c.CallUnary(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	require.NoError(t, err)

	capTransport.mu.Lock()
	defer capTransport.mu.Unlock()

	assert.Equal(t, grpcutil.Brotli, capTransport.requestHdr.Get("Grpc-Encoding"))
	assert.Equal(t, grpcutil.Brotli, capTransport.responseHdr.Get("Grpc-Encoding"))
}
