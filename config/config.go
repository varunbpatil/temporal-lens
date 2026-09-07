// Package config provides application configuration parsed from environment variables.
package config

import (
	"fmt"
	"net/url"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Log        LogConfig        `envPrefix:"LOG_"`
	GRPC       GRPCConfig       `envPrefix:"GRPC_"`
	HTTP       HTTPConfig       `envPrefix:"HTTP_"`
	OpenSearch OpenSearchConfig `envPrefix:"OPENSEARCH_"`
	Temporal   TemporalConfig   `envPrefix:"TEMPORAL_"`
}

type TemporalConfig struct {
	Namespaces               []string `env:"NAMESPACES,required"`
	Cloud                    bool     `env:"CLOUD"`
	Account                  string   `env:"ACCOUNT"`
	Endpoint                 string   `env:"ENDPOINT"`
	BaseURL                  url.URL  `env:"BASE_URL,required"`
	APIKey                   string   `env:"API_KEY"`
	TLS                      bool     `env:"TLS"`
	InsecureSkipVerify       bool     `env:"INSECURE_SKIP_VERIFY"`
	ServerName               string   `env:"SERVER_NAME"`
	CAFile                   string   `env:"CA_FILE"`
	ClientCertFile           string   `env:"CLIENT_CERT_FILE"`
	ClientKeyFile            string   `env:"CLIENT_KEY_FILE"`
	ConnPoolSize             int      `env:"CONN_POOL_SIZE"              envDefault:"1"`
	BulkActionsPerSecond     int      `env:"BULK_ACTIONS_PER_SECOND"     envDefault:"8"`
	ListRequestsPerSecond    int      `env:"LIST_REQUESTS_PER_SECOND"    envDefault:"8"`
	HistoryRequestsPerSecond int      `env:"HISTORY_REQUESTS_PER_SECOND" envDefault:"8"`
}

type LogConfig struct {
	Level  string `env:"LEVEL"  envDefault:"info"`
	Format string `env:"FORMAT" envDefault:"text"`
}

type GRPCConfig struct {
	Address string `env:"ADDRESS" envDefault:":50051"`
}

type HTTPConfig struct {
	Address string `env:"ADDRESS" envDefault:":8080"`
}

type OpenSearchConfig struct {
	Addresses          []string `env:"ADDRESSES,required"`
	Username           string   `env:"USERNAME"`
	Password           string   `env:"PASSWORD"`
	APIKey             string   `env:"API_KEY"`
	InsecureSkipVerify bool     `env:"INSECURE_SKIP_VERIFY"`
}

func Parse() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &cfg, nil
}
