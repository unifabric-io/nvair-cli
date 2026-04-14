package create

import (
	"fmt"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
)

func ensureAuthenticatedClient(apiEndpoint string) (*api.Client, *config.Config, error) {
	logging.Verbose("Checking authentication status")
	cfg, err := config.Load()
	if err != nil || cfg.BearerToken == "" {
		logging.Verbose("Not authenticated")
		return nil, nil, fmt.Errorf("not authenticated. Please run 'nvair login' first")
	}
	logging.Verbose("Authentication verified")

	endpoint := config.ResolveAPIEndpoint(cfg, apiEndpoint)

	logging.Verbose("Checking token expiration")
	if cfg.IsTokenExpired(time.Now()) {
		logging.Verbose("Bearer token has expired, attempting to refresh with saved API token")

		if cfg.APIToken == "" {
			logging.Verbose("No saved API token available for refresh")
			return nil, nil, fmt.Errorf("authentication token has expired and no API token available. Please run 'nvair login' again")
		}

		apiClient := api.NewClient(endpoint, "")
		newBearerToken, expiresAt, err := apiClient.AuthLogin(cfg.Username, cfg.APIToken)
		if err != nil {
			logging.Verbose("Failed to refresh token: %v", err)
			return nil, nil, fmt.Errorf("authentication token expired and refresh failed: %w", err)
		}

		logging.Verbose("Successfully refreshed bearer token")
		cfg.BearerToken = newBearerToken
		cfg.BearerTokenExpiresAt = expiresAt

		if err := cfg.Save(); err != nil {
			logging.Verbose("Warning: Failed to save refreshed token: %v", err)
		}
	}
	logging.Verbose("Token is valid")

	apiClient := api.NewClient(endpoint, cfg.BearerToken)
	return apiClient, cfg, nil
}
