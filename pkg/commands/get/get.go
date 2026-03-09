package get

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/node"
	"github.com/unifabric-io/nvair-cli/pkg/output"
)

const (
	formatDefault = "default"
	formatJSON    = "json"
	formatYAML    = "yaml"
)

// NodeCount holds per-simulation node type counts.
type NodeCount struct {
	Switch int `json:"switch" yaml:"switch"`
	Host   int `json:"host"   yaml:"host"`
}

// NodeImageView is the resolved image representation used in NodeView.
type NodeImageView struct {
	ID   string `json:"id"             yaml:"id"`
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}

// NodeView is the enriched node representation for `get nodes` output.
type NodeView struct {
	ID         string             `json:"id"                 yaml:"id"`
	Name       string             `json:"name"               yaml:"name"`
	State      string             `json:"state"              yaml:"state"`
	Simulation string             `json:"simulation"         yaml:"simulation"`
	Image      NodeImageView      `json:"image"              yaml:"image"`
	Metadata   *node.NodeMetadata `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// SimulationSummary is the enriched view of a simulation used in `get simulations` output.
type SimulationSummary struct {
	ID    string    `json:"id"    yaml:"id"`
	Title string    `json:"title" yaml:"title"`
	State string    `json:"state" yaml:"state"`
	Count NodeCount `json:"count" yaml:"count"`
}

// Command represents the get subcommand.
type Command struct {
	APIEndpoint    string
	OutputFormat   string
	SimulationName string
	Verbose        bool
}

// NewCommand creates a new get command with defaults.
func NewCommand() *Command {
	return &Command{APIEndpoint: "https://air.nvidia.com/api"}
}

// Register registers get subcommands and flags.
func (gc *Command) Register(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&gc.APIEndpoint, "api-endpoint", gc.APIEndpoint, "API endpoint URL")

	simCmd := &cobra.Command{
		Use:     "simulations",
		Aliases: []string{"simulation"},
		Short:   "List simulations",
		RunE: func(cmd *cobra.Command, args []string) error {
			gc.OutputFormat = strings.TrimSpace(gc.OutputFormat)
			return gc.executeSimulations(cmd)
		},
	}
	simCmd.Flags().StringVarP(&gc.OutputFormat, "output", "o", "", "Output format: json|yaml")

	nodesCmd := &cobra.Command{
		Use:     "nodes",
		Aliases: []string{"node"},
		Short:   "List nodes in a simulation",
		RunE: func(cmd *cobra.Command, args []string) error {
			gc.OutputFormat = strings.TrimSpace(gc.OutputFormat)
			gc.SimulationName = strings.TrimSpace(gc.SimulationName)
			return gc.executeNodes(cmd)
		},
	}
	nodesCmd.Flags().StringVarP(&gc.SimulationName, "simulation", "s", "", "Simulation name (required)")
	nodesCmd.Flags().StringVarP(&gc.OutputFormat, "output", "o", "", "Output format: json|yaml")

	cmd.AddCommand(simCmd, nodesCmd)
}

func (gc *Command) executeSimulations(cmd *cobra.Command) error {
	gc.configureVerbose()

	format, err := normalizeOutputFormat(gc.OutputFormat)
	if err != nil {
		return err
	}

	apiClient, _, err := ensureAuthenticatedClient(gc.APIEndpoint)
	if err != nil {
		return err
	}

	simulations, err := apiClient.GetSimulations()
	if err != nil {
		return err
	}

	allNodes, err := apiClient.GetAllNodes()
	if err != nil {
		return err
	}

	images, err := apiClient.GetImages()
	if err != nil {
		return err
	}

	summaries := buildSimulationSummaries(simulations, allNodes, images)
	return renderSimulationOutput(cmd.OutOrStdout(), summaries, format)
}

func (gc *Command) executeNodes(cmd *cobra.Command) error {
	gc.configureVerbose()

	if gc.SimulationName == "" {
		return output.NewValidationError("--simulation <name> is required")
	}

	format, err := normalizeOutputFormat(gc.OutputFormat)
	if err != nil {
		return err
	}

	apiClient, _, err := ensureAuthenticatedClient(gc.APIEndpoint)
	if err != nil {
		return err
	}

	simulationID, err := gc.resolveSimulationID(apiClient, gc.SimulationName)
	if err != nil {
		return err
	}

	nodes, err := apiClient.GetNodes(simulationID)
	if err != nil {
		return err
	}

	images, err := apiClient.GetImages()
	if err != nil {
		return err
	}

	views := buildNodeViews(nodes, images)
	views = filterOOBNodes(views)
	return renderNodeOutput(cmd.OutOrStdout(), views, format)
}

func (gc *Command) resolveSimulationID(apiClient *api.Client, simulationName string) (string, error) {
	simulations, err := apiClient.GetSimulations()
	if err != nil {
		return "", err
	}

	for _, sim := range simulations {
		if sim.Title == simulationName {
			return sim.ID, nil
		}
	}

	return "", output.NewNotFoundError(fmt.Sprintf("simulation not found: %s", simulationName))
}

func (gc *Command) configureVerbose() {
	if gc.Verbose {
		logging.SetVerbose(os.Stderr)
		logging.Verbose("Verbose mode enabled")
	}
}

func normalizeOutputFormat(outputFormat string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "":
		return formatDefault, nil
	case formatJSON:
		return formatJSON, nil
	case formatYAML:
		return formatYAML, nil
	default:
		return "", output.NewValidationError(fmt.Sprintf("unsupported output format: %s (supported: json, yaml)", outputFormat))
	}
}

func renderSimulationOutput(w io.Writer, summaries []SimulationSummary, format string) error {
	switch format {
	case formatJSON:
		return writeJSON(w, summaries)
	case formatYAML:
		return writeYAML(w, summaries)
	default:
		if len(summaries) == 0 {
			_, err := fmt.Fprintln(w, "No simulations found")
			return err
		}

		rows := make([][]string, 0, len(summaries))
		for _, s := range summaries {
			rows = append(rows, []string{
				s.Title,
				s.State,
				s.ID,
				fmt.Sprintf("%d", s.Count.Switch),
				fmt.Sprintf("%d", s.Count.Host),
			})
		}
		return writeTable(w, []string{"NAME", "STATUS", "ID", "SWITCH", "HOST"}, rows)
	}
}

func buildSimulationSummaries(simulations []api.SimulationInfo, allNodes []api.Node, images []api.ImageInfo) []SimulationSummary {
	imageNames := make(map[string]string, len(images))
	for _, img := range images {
		imageNames[img.ID] = img.Name
	}

	countMap := make(map[string]*NodeCount, len(simulations))
	for _, sim := range simulations {
		countMap[sim.ID] = &NodeCount{}
	}

	for _, node := range allNodes {
		counts, ok := countMap[node.Simulation]
		if !ok {
			continue
		}
		imgName := strings.ToLower(imageNames[node.OS])
		if imgName == "" {
			imgName = strings.ToLower(node.OS)
		}
		if strings.Contains(imgName, "cumulus") {
			counts.Switch++
		} else if strings.Contains(imgName, "generic") || strings.Contains(imgName, "ubuntu") {
			counts.Host++
		}
	}

	summaries := make([]SimulationSummary, 0, len(simulations))
	for _, sim := range simulations {
		counts := countMap[sim.ID]
		if counts == nil {
			counts = &NodeCount{}
		}
		summaries = append(summaries, SimulationSummary{
			ID:    sim.ID,
			Title: sim.Title,
			State: sim.State,
			Count: *counts,
		})
	}
	return summaries
}

func renderNodeOutput(w io.Writer, nodes []NodeView, format string) error {
	switch format {
	case formatJSON:
		return writeJSON(w, nodes)
	case formatYAML:
		return writeYAML(w, nodes)
	default:
		if len(nodes) == 0 {
			_, err := fmt.Fprintln(w, "No nodes found")
			return err
		}

		rows := make([][]string, 0, len(nodes))
		for _, node := range nodes {
			mgmtIP := "-"
			if node.Metadata != nil && node.Metadata.MgmtIP != "" {
				mgmtIP = node.Metadata.MgmtIP
			}
			osDisplay := node.Image.Name
			if osDisplay == "" {
				osDisplay = node.Image.ID
			}
			rows = append(rows, []string{node.Name, node.State, mgmtIP, osDisplay})
		}
		return writeTable(w, []string{"NAME", "STATUS", "MGMT_IP", "IMAGE"}, rows)
	}
}

func buildNodeViews(nodes []api.Node, images []api.ImageInfo) []NodeView {
	imageMap := make(map[string]api.ImageInfo, len(images))
	for _, img := range images {
		imageMap[img.ID] = img
	}

	views := make([]NodeView, 0, len(nodes))
	for _, n := range nodes {
		imgView := NodeImageView{ID: n.OS}
		if img, ok := imageMap[n.OS]; ok {
			imgView.Name = img.Name
		}

		var meta *node.NodeMetadata
		if n.Metadata != "" {
			if parsed, err := node.ParseNodeMetadata(n.Metadata); err == nil {
				meta = parsed
			}
		}

		views = append(views, NodeView{
			ID:         n.ID,
			Name:       n.Name,
			State:      n.State,
			Simulation: n.Simulation,
			Image:      imgView,
			Metadata:   meta,
		})
	}
	return views
}

func filterOOBNodes(views []NodeView) []NodeView {
	filtered := views[:0]
	for _, v := range views {
		if v.Name != constant.OOBMgmtServerName && v.Name != constant.OOBMgmtSwitchName {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

func extractMgmtIP(metadata string) string {
	metadata = strings.TrimSpace(metadata)
	if metadata == "" {
		return "-"
	}

	var metaMap map[string]interface{}
	if err := json.Unmarshal([]byte(metadata), &metaMap); err != nil {
		return "-"
	}

	ip, ok := metaMap["mgmt_ip"].(string)
	if !ok || strings.TrimSpace(ip) == "" {
		return "-"
	}

	return ip
}

func writeTable(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintln(tw, strings.Join(headers, "\t")); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
			return err
		}
	}

	return tw.Flush()
}

func writeJSON(w io.Writer, v interface{}) error {
	payload, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal output as json: %w", err)
	}
	_, err = fmt.Fprintln(w, string(payload))
	return err
}

func writeYAML(w io.Writer, v interface{}) error {
	payload, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal output as yaml: %w", err)
	}
	_, err = fmt.Fprint(w, string(payload))
	return err
}

func ensureAuthenticatedClient(apiEndpoint string) (*api.Client, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil || cfg.BearerToken == "" {
		return nil, nil, fmt.Errorf("not authenticated. Please run 'nvair login' first")
	}

	if cfg.IsTokenExpired(time.Now()) {
		if cfg.APIToken == "" {
			return nil, nil, fmt.Errorf("authentication token has expired and no API token available. Please run 'nvair login' again")
		}

		refreshClient := api.NewClient(apiEndpoint, "")
		newBearerToken, expiresAt, err := refreshClient.AuthLogin(cfg.Username, cfg.APIToken)
		if err != nil {
			return nil, nil, fmt.Errorf("authentication token expired and refresh failed: %w", err)
		}

		cfg.BearerToken = newBearerToken
		cfg.BearerTokenExpiresAt = expiresAt
		if err := cfg.Save(); err != nil {
			logging.Verbose("Warning: Failed to save refreshed token: %v", err)
			return nil, nil, fmt.Errorf("authentication token refreshed but failed to persist new token: %w", err)
		}
	}

	return api.NewClient(apiEndpoint, cfg.BearerToken), cfg, nil
}
