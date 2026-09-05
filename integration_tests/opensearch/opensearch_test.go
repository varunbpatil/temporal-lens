//go:build integration || all

package opensearch_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

//nolint:gochecknoglobals // Shared across test suite to avoid passing through every test.
var sharedAddr string

func TestMain(m *testing.M) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "opensearchproject/opensearch:3.8.0",
		ExposedPorts: []string{"9200/tcp"},
		Env: map[string]string{
			"discovery.type":              "single-node",
			"DISABLE_SECURITY_PLUGIN":     "true",
			"DISABLE_INSTALL_DEMO_CONFIG": "true",
			"OPENSEARCH_JAVA_OPTS":        "-Xms256m -Xmx256m",
		},
		WaitingFor: wait.ForHTTP("/_cluster/health").
			WithPort("9200").
			WithStartupTimeout(120 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "start container: %v\n", err)
		os.Exit(1)
	}

	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get host: %v\n", err)
		os.Exit(1)
	}
	port, err := container.MappedPort(ctx, "9200")
	if err != nil {
		fmt.Fprintf(os.Stderr, "get port: %v\n", err)
		os.Exit(1)
	}

	sharedAddr = fmt.Sprintf("http://%s", net.JoinHostPort(host, port.Port()))
	createDefaultIndex()
	code := m.Run()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

func createDefaultIndex() {
	mapping := `{"mappings":{"properties":{` +
		`"id":{"type":"keyword"},` +
		`"status":{"type":"keyword"},` +
		`"name":{"type":"text"},` +
		`"attempts":{"type":"integer"},` +
		`"paused":{"type":"boolean"},` +
		`"score":{"type":"double"},` +
		`"startTime":{"type":"date"},` +
		`"endTime":{"type":"date"},` +
		`"namespace":{"type":"keyword"}` +
		`}}}`
	req, err := http.NewRequest(http.MethodPut, sharedAddr+"/workflows", bytes.NewBufferString(mapping))
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		panic(fmt.Sprintf("create index failed (%d): %s", resp.StatusCode, body))
	}
}

func refreshIndex(name string) {
	req, err := http.NewRequest(http.MethodPost, sharedAddr+"/"+name+"/_refresh", nil)
	if err != nil {
		panic(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	resp.Body.Close()
}
