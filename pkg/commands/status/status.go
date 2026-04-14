package status

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
)

// Command reports the current login and connectivity state.
type Command struct {
	APIEndpoint string
	Verbose     bool
}

type statusResult struct {
	Username   string
	Endpoint   string
	LoggedIn   bool
	CanConnect bool
}

// NewCommand creates a new status command with defaults.
func NewCommand() *Command {
	return &Command{
		APIEndpoint: constant.DefaultAPIEndpoint,
	}
}

// Register registers status flags.
func (sc *Command) Register(cmd *cobra.Command) {
	_ = cmd
}

// Execute runs the status command.
func (sc *Command) Execute(cmd *cobra.Command) error {
	sc.configureVerbose()

	result := sc.resolveStatus()
	return writeStatus(cmd.OutOrStdout(), result)
}

func (sc *Command) configureVerbose() {
	if sc.Verbose {
		logging.SetVerbose(os.Stderr)
		logging.Verbose("Verbose mode enabled")
	}
}

func (sc *Command) resolveStatus() statusResult {
	loadedCfg := sc.loadConfig()
	endpoint := displayEndpoint(config.ResolveAPIEndpoint(loadedCfg, sc.APIEndpoint))

	cfg := sc.loadUsableSession(loadedCfg)
	if cfg == nil {
		return statusResult{
			Endpoint: endpoint,
		}
	}

	if err := sc.probeConnectivity(cfg); err != nil {
		logging.Verbose("Status: connectivity probe failed for %s: %v", cfg.Username, err)
		return statusResult{
			Username:   cfg.Username,
			Endpoint:   endpoint,
			LoggedIn:   true,
			CanConnect: false,
		}
	}

	return statusResult{
		Username:   cfg.Username,
		Endpoint:   endpoint,
		LoggedIn:   true,
		CanConnect: true,
	}
}

func (sc *Command) loadConfig() *config.Config {
	cfg, err := config.Load()
	if err != nil {
		logging.Verbose("Status: unable to load configuration: %v", err)
		return nil
	}

	return cfg
}

func (sc *Command) loadUsableSession(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}

	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.APIToken = strings.TrimSpace(cfg.APIToken)
	cfg.BearerToken = strings.TrimSpace(cfg.BearerToken)

	if cfg.Username == "" || cfg.BearerToken == "" {
		logging.Verbose("Status: configuration is missing required session fields")
		return nil
	}

	if !cfg.IsTokenExpired(time.Now()) {
		return cfg
	}

	if cfg.APIToken == "" {
		logging.Verbose("Status: bearer token expired and no API token is available for refresh")
		return nil
	}

	endpoint := config.ResolveAPIEndpoint(cfg, sc.APIEndpoint)
	refreshClient := api.NewClient(endpoint, "")
	newBearerToken, expiresAt, err := refreshClient.AuthLogin(cfg.Username, cfg.APIToken)
	if err != nil {
		logging.Verbose("Status: bearer token refresh failed: %v", err)
		return nil
	}

	cfg.BearerToken = strings.TrimSpace(newBearerToken)
	cfg.BearerTokenExpiresAt = expiresAt
	if cfg.BearerToken == "" {
		logging.Verbose("Status: refresh returned an empty bearer token")
		return nil
	}

	if err := cfg.Save(); err != nil {
		logging.Verbose("Status: refreshed token could not be saved: %v", err)
	}

	return cfg
}

func (sc *Command) probeConnectivity(cfg *config.Config) error {
	apiClient := api.NewClient(config.ResolveAPIEndpoint(cfg, sc.APIEndpoint), cfg.BearerToken)
	_, err := apiClient.GetSimulations()
	return err
}

func writeStatus(w io.Writer, result statusResult) error {
	user := "Not logged in"
	access := "No"
	if result.LoggedIn {
		user = result.Username
		if result.CanConnect {
			access = "Yes"
		}
	}

	return writeStatusLine(w, "User", user, "Endpoint", result.Endpoint, "Access", access)
}

func writeStatusLine(w io.Writer, label1, value1, label2, value2, label3, value3 string) error {
	_, err := fmt.Fprintf(w, "%-14s: %s\n%-14s: %s\n%-14s: %s\n", label1, value1, label2, value2, label3, value3)
	return err
}

func displayEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err == nil && parsed.Host != "" {
		return parsed.Host
	}

	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimSuffix(raw, "/")
	if idx := strings.Index(raw, "/"); idx >= 0 {
		raw = raw[:idx]
	}

	return raw
}
