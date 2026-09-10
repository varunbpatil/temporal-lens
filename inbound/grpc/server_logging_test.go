//nolint:testpackage // The logging interceptor is intentionally internal to the gRPC adapter.
package grpc

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
)

func TestLoggingInterceptorLogsHandlerErrors(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	interceptor := loggingInterceptor(logger)
	next := interceptor.WrapUnary(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, connect.NewError(connect.CodeInternal, errors.New("Temporal rejected reset"))
	})

	_, err := next(t.Context(), connect.NewRequest(new(struct{})))

	require.Error(t, err)
	require.Contains(t, logs.String(), "msg=\"RPC failed\"")
	require.Contains(t, logs.String(), "code=internal")
	require.Contains(t, logs.String(), "Temporal rejected reset")
}
