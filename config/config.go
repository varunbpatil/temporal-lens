// Package config provides application configuration parsed from environment variables.
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Log        LogConfig        `envPrefix:"LOG_"`
	OpenSearch OpenSearchConfig `envPrefix:"OPENSEARCH_"`
}

type LogConfig struct {
	Level  string `env:"LEVEL"  envDefault:"info"`
	Format string `env:"FORMAT" envDefault:"text"`
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
