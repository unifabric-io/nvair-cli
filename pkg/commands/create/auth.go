package create

import (
	"fmt"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
)

func ensureAuthenticatedClient(apiEndpoint string) (*api.Client, *config.Config, error) {
	logging.Verbose("Checking authentication status")
	cfg, err := config.Load()
	if err != nil || cfg.APIToken == "" {
		logging.Verbose("Not authenticated")
		return nil, nil, fmt.Errorf("not authenticated. Please run 'nvair login' first")
	}
	logging.Verbose("Authentication verified")

	apiClient := api.NewClient(apiEndpoint, cfg.APIToken)
	return apiClient, cfg, nil
}
