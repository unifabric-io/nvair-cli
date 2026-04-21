package get

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/bastion"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
	forwardutil "github.com/unifabric-io/nvair-cli/pkg/forward"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/node"
	"github.com/unifabric-io/nvair-cli/pkg/output"
	"github.com/unifabric-io/nvair-cli/pkg/simulation"
	sshpkg "github.com/unifabric-io/nvair-cli/pkg/ssh"
)

const (
	formatDefault = "default"
	formatJSON    = "json"
	formatYAML    = "yaml"
)

const managedForwardListenPortStart = 20000

var (
	defaultKeyPathFn       = sshpkg.DefaultKeyPath
	execCommandOnBastionFn = bastion.ExecCommandOnBastion
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
	ID      string    `json:"id"      yaml:"id"`
	Title   string    `json:"title"   yaml:"title"`
	State   string    `json:"state"   yaml:"state"`
	Created string    `json:"created" yaml:"created"`
	Count   NodeCount `json:"count"   yaml:"count"`
}

// ForwardView is the representation for `get forward` output.
type ForwardView struct {
	ID          string `json:"id"           yaml:"id"`
	Name        string `json:"name"         yaml:"name"`
	ServiceType string `json:"service_type" yaml:"service_type"`
	NodeName    string `json:"node_name"    yaml:"node_name"`
	DestPort    int    `json:"dest_port"    yaml:"dest_port"`
	SrcPort     int    `json:"src_port"     yaml:"src_port"`
	Host        string `json:"host"         yaml:"host"`
	Link        string `json:"link"         yaml:"link"`
	Address     string `json:"address"      yaml:"address"`
	TargetHost  string `json:"target_host,omitempty" yaml:"target_host,omitempty"`
	TargetPort  int    `json:"target_port,omitempty" yaml:"target_port,omitempty"`
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
	nodesCmd.Flags().StringVarP(&gc.SimulationName, "simulation", "s", "", "Simulation name (optional when only one simulation exists)")
	nodesCmd.Flags().StringVarP(&gc.OutputFormat, "output", "o", "", "Output format: json|yaml")

	forwardCmd := &cobra.Command{
		Use:     "forward",
		Aliases: []string{"forwards"},
		Short:   "List forward services in a simulation",
		RunE: func(cmd *cobra.Command, args []string) error {
			gc.OutputFormat = strings.TrimSpace(gc.OutputFormat)
			gc.SimulationName = strings.TrimSpace(gc.SimulationName)
			return gc.executeForward(cmd)
		},
	}
	forwardCmd.Flags().StringVarP(&gc.SimulationName, "simulation", "s", "", "Simulation name (optional when only one simulation exists)")
	forwardCmd.Flags().StringVarP(&gc.OutputFormat, "output", "o", "", "Output format: json|yaml")

	cmd.AddCommand(simCmd, nodesCmd, forwardCmd)
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

	format, err := normalizeOutputFormat(gc.OutputFormat)
	if err != nil {
		return err
	}

	apiClient, _, err := ensureAuthenticatedClient(gc.APIEndpoint)
	if err != nil {
		return err
	}

	resolvedSimulation, err := simulation.Resolve(apiClient, gc.SimulationName)
	if err != nil {
		return err
	}
	if resolvedSimulation.AutoSelected {
		_ = simulation.WriteDefaultSelectionNotice(cmd.ErrOrStderr(), resolvedSimulation.Name)
	}

	nodes, err := apiClient.GetNodes(resolvedSimulation.ID)
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

func (gc *Command) executeForward(cmd *cobra.Command) error {
	gc.configureVerbose()

	format, err := normalizeOutputFormat(gc.OutputFormat)
	if err != nil {
		return err
	}

	apiClient, _, err := ensureAuthenticatedClient(gc.APIEndpoint)
	if err != nil {
		return err
	}

	resolvedSimulation, err := simulation.Resolve(apiClient, gc.SimulationName)
	if err != nil {
		return err
	}
	if resolvedSimulation.AutoSelected {
		_ = simulation.WriteDefaultSelectionNotice(cmd.ErrOrStderr(), resolvedSimulation.Name)
	}

	services, err := apiClient.GetServices(resolvedSimulation.ID)
	if err != nil {
		return err
	}

	forwards := buildForwardViews(services)
	if shouldInspectForwardTargets(services) {
		forwards = gc.enrichForwardTargetsFromIPTables(apiClient, resolvedSimulation.ID, forwards)
	}
	return renderForwardOutput(cmd.OutOrStdout(), forwards, format)
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
			created := s.Created
			if t, err := time.Parse(time.RFC3339Nano, s.Created); err == nil {
				created = t.Local().Format("2006-01-02 15:04:05")
			}
			rows = append(rows, []string{
				s.Title,
				s.State,
				created,
				s.ID,
				fmt.Sprintf("%d", s.Count.Switch),
				fmt.Sprintf("%d", s.Count.Host),
			})
		}
		return writeTable(w, []string{"NAME", "STATUS", "CREATED", "ID", "SWITCH", "HOST"}, rows)
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
			ID:      sim.ID,
			Title:   sim.Title,
			State:   sim.State,
			Created: sim.Created,
			Count:   *counts,
		})
	}
	return summaries
}

func buildForwardViews(services []api.EnableSSHResponse) []ForwardView {
	views := make([]ForwardView, 0, len(services))
	for _, service := range services {
		targetHost, targetPort := resolveForwardTarget(service)
		views = append(views, ForwardView{
			ID:          service.ID,
			Name:        service.Name,
			ServiceType: service.ServiceType,
			NodeName:    service.NodeName,
			DestPort:    service.DestPort,
			SrcPort:     service.SrcPort,
			Host:        service.Host,
			Link:        service.Link,
			Address:     resolveForwardAddress(service),
			TargetHost:  targetHost,
			TargetPort:  targetPort,
		})
	}

	sort.SliceStable(views, func(i, j int) bool {
		if views[i].Name == views[j].Name {
			return views[i].SrcPort < views[j].SrcPort
		}
		return views[i].Name < views[j].Name
	})

	return views
}

func resolveForwardTarget(service api.EnableSSHResponse) (string, int) {
	return service.NodeName, service.DestPort
}

func resolveForwardAddress(service api.EnableSSHResponse) string {
	if link := strings.TrimSpace(service.Link); link != "" {
		return link
	}

	host := strings.TrimSpace(service.Host)
	switch {
	case host != "" && service.SrcPort > 0:
		return net.JoinHostPort(host, strconv.Itoa(service.SrcPort))
	case host != "":
		return host
	case service.SrcPort > 0:
		return strconv.Itoa(service.SrcPort)
	default:
		return "-"
	}
}

func shouldInspectForwardTargets(services []api.EnableSSHResponse) bool {
	for _, service := range services {
		if service.DestPort < managedForwardListenPortStart {
			continue
		}
		if strings.EqualFold(service.ServiceType, "ssh") {
			continue
		}
		return true
	}
	return false
}

func (gc *Command) enrichForwardTargetsFromIPTables(apiClient *api.Client, simulationID string, forwards []ForwardView) []ForwardView {
	targets, err := gc.inspectForwardTargets(apiClient, simulationID)
	if err != nil {
		logging.Verbose("Skipping iptables target inspection: %v", err)
		return forwards
	}
	if len(targets) == 0 {
		return forwards
	}

	nodeNamesByIP := gc.nodeNamesByMgmtIP(apiClient, simulationID)
	for i := range forwards {
		target, ok := targets[forwards[i].DestPort]
		if !ok {
			continue
		}

		targetHost := target.Host
		if nodeName, ok := nodeNamesByIP[targetHost]; ok {
			targetHost = nodeName
		}
		forwards[i].TargetHost = targetHost
		forwards[i].TargetPort = target.Port
	}

	return forwards
}

func (gc *Command) inspectForwardTargets(apiClient *api.Client, simulationID string) (map[int]forwardutil.IPTablesTarget, error) {
	result, err := gc.runCommandOnBastion(apiClient, simulationID, "sudo iptables-save -t nat 2>/dev/null || sudo iptables -t nat -S")
	if err != nil {
		return nil, err
	}
	if result != nil && result.ExitCode != 0 {
		return nil, fmt.Errorf("iptables inspection exited with code %d: %s", result.ExitCode, result.Stderr)
	}

	if result == nil {
		return map[int]forwardutil.IPTablesTarget{}, nil
	}
	return forwardutil.ParseIPTablesTargets(result.Stdout), nil
}

func (gc *Command) nodeNamesByMgmtIP(apiClient *api.Client, simulationID string) map[string]string {
	namesByIP := make(map[string]string)

	nodes, err := apiClient.GetNodes(simulationID)
	if err != nil {
		logging.Verbose("Skipping node IP lookup for forward targets: %v", err)
		return namesByIP
	}
	for _, n := range nodes {
		metadata, err := node.ParseNodeMetadata(n.Metadata)
		if err != nil || metadata.MgmtIP == "" {
			continue
		}
		namesByIP[strings.TrimSpace(metadata.MgmtIP)] = n.Name
	}

	return namesByIP
}

func (gc *Command) runCommandOnBastion(apiClient *api.Client, simulationID, command string) (*bastion.ExecResult, error) {
	sshHost, sshPort, err := gc.findSSHService(apiClient, simulationID)
	if err != nil {
		return nil, err
	}

	keyPath, err := defaultKeyPathFn()
	if err != nil {
		return nil, fmt.Errorf("failed to get SSH key path: %w", err)
	}

	cfg := bastion.BastionExecConfig{
		BastionUser: constant.DefaultUbuntuUser,
		BastionAddr: net.JoinHostPort(sshHost, strconv.Itoa(sshPort)),
		BastionKey:  keyPath,
		TargetUser:  constant.DefaultUbuntuUser,
		Command:     command,
	}

	return execCommandOnBastionFn(cfg)
}

func (gc *Command) findSSHService(apiClient *api.Client, simulationID string) (string, int, error) {
	services, err := apiClient.GetServices(simulationID)
	if err != nil {
		return "", 0, err
	}
	for _, svc := range services {
		if strings.EqualFold(svc.ServiceType, "ssh") &&
			svc.Host != "" &&
			svc.SrcPort > 0 {
			return svc.Host, svc.SrcPort, nil
		}
	}
	return "", 0, fmt.Errorf("SSH service not found; run 'nvair create' first")
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

func renderForwardOutput(w io.Writer, forwards []ForwardView, format string) error {
	switch format {
	case formatJSON:
		return writeJSON(w, forwards)
	case formatYAML:
		return writeYAML(w, forwards)
	default:
		if len(forwards) == 0 {
			_, err := fmt.Fprintln(w, "No forwards found")
			return err
		}

		rows := make([][]string, 0, len(forwards))
		for _, forward := range forwards {
			// Strip scheme and userinfo from address to get plain host:port
			external := forward.Address
			if idx := strings.Index(external, "://"); idx >= 0 {
				external = external[idx+3:]
			}
			if idx := strings.Index(external, "@"); idx >= 0 {
				external = external[idx+1:]
			}
			external = strings.TrimRight(external, "/")
			if external == "" {
				external = "-"
			}

			destHost := forward.TargetHost
			if destHost == "" {
				destHost = forward.NodeName
			}
			destPort := forward.TargetPort
			if destPort == 0 {
				destPort = forward.DestPort
			}
			dest := destHost
			if destHost != "" && destPort > 0 {
				dest = net.JoinHostPort(destHost, strconv.Itoa(destPort))
			}

			rows = append(rows, []string{
				forward.Name,
				external,
				dest,
			})
		}
		return writeTable(w, []string{"NAME", "EXTERNAL", "TARGET"}, rows)
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
