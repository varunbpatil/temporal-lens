package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/varunbpatil/temporal-lens/config"
)

func TestParse(t *testing.T) {
	t.Setenv("OPENSEARCH_ADDRESSES", "https://search.example.com:9200")
	t.Setenv("OPENSEARCH_USERNAME", "admin")
	t.Setenv("OPENSEARCH_PASSWORD", "secret")
	t.Setenv("OPENSEARCH_API_KEY", "os_abc123")
	t.Setenv("OPENSEARCH_INSECURE_SKIP_VERIFY", "true")

	cfg, err := config.Parse()
	require.NoError(t, err)
	assert.Equal(t, []string{"https://search.example.com:9200"}, cfg.OpenSearch.Addresses)
	assert.Equal(t, "admin", cfg.OpenSearch.Username)
	assert.Equal(t, "secret", cfg.OpenSearch.Password)
	assert.Equal(t, "os_abc123", cfg.OpenSearch.APIKey)
	assert.True(t, cfg.OpenSearch.InsecureSkipVerify)
}

func TestParse_MultipleAddresses(t *testing.T) {
	t.Setenv("OPENSEARCH_ADDRESSES", "https://node1:9200,https://node2:9200,https://node3:9200")
	t.Setenv("OPENSEARCH_USERNAME", "")
	t.Setenv("OPENSEARCH_PASSWORD", "")
	t.Setenv("OPENSEARCH_API_KEY", "os_abc123")
	t.Setenv("OPENSEARCH_INSECURE_SKIP_VERIFY", "false")

	cfg, err := config.Parse()
	require.NoError(t, err)
	assert.Equal(t, []string{
		"https://node1:9200",
		"https://node2:9200",
		"https://node3:9200",
	}, cfg.OpenSearch.Addresses)
	assert.Equal(t, "os_abc123", cfg.OpenSearch.APIKey)
	assert.Empty(t, cfg.OpenSearch.Username)
	assert.Empty(t, cfg.OpenSearch.Password)
}
