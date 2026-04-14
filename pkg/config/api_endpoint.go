package config

import (
	"strings"

	"github.com/unifabric-io/nvair-cli/pkg/constant"
)

// ResolveAPIEndpoint returns the configured API endpoint when present.
// It falls back to the provided value, then to the project default.
func ResolveAPIEndpoint(cfg *Config, fallback string) string {
	if cfg != nil {
		if endpoint := strings.TrimSpace(cfg.APIEndpoint); endpoint != "" {
			return endpoint
		}
	}

	if endpoint := strings.TrimSpace(fallback); endpoint != "" {
		return endpoint
	}

	return constant.DefaultAPIEndpoint
}
