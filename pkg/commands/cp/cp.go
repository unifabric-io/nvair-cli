package cp

import (
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
	forwardutil "github.com/unifabric-io/nvair-cli/pkg/forward"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/node"
	"github.com/unifabric-io/nvair-cli/pkg/output"
	"github.com/unifabric-io/nvair-cli/pkg/simulation"
	sshpkg "github.com/unifabric-io/nvair-cli/pkg/ssh"
)

var (
	copyFileViaBastionFn  = sshpkg.CopyFileViaBastion
	copyFileFromBastionFn = sshpkg.CopyFileFromBastion
	defaultKeyPathFn      = sshpkg.DefaultKeyPath
)

const defaultCopyTimeout = 2 * time.Minute

// Command represents the cp subcommand.
type Command struct {
	SimulationName string
	APIEndpoint    string
	Timeout        time.Duration
	Stderr         io.Writer
	Verbose        bool
}

type copyRemotePath struct {
	NodeName string
	Path     string
}

type resolvedCredentials struct {
	TargetUser    string
	TargetPass    string
	DirectBastion bool
}

// NewCommand creates a new cp command with defaults.
func NewCommand() *Command {
	return &Command{
		APIEndpoint: constant.DefaultAPIEndpoint,
		Timeout:     defaultCopyTimeout,
	}
}

// Register registers cp flags.
func (cc *Command) Register(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVarP(&cc.SimulationName, "simulation", "s", cc.SimulationName, "Simulation name (optional when only one simulation exists)")
	flags.DurationVar(&cc.Timeout, "timeout", cc.Timeout, "Copy timeout (e.g. 30s, 2m)")
}

// Execute runs the cp command.
func (cc *Command) Execute(args []string) error {
	cc.configureVerbose()

	if len(args) != 2 {
		return output.NewUsageError("cp requires exactly two arguments: <src> <dest>")
	}

	srcRemote, srcIsRemote, err := parseCopyLocation(args[0])
	if err != nil {
		return err
	}
	dstRemote, dstIsRemote, err := parseCopyLocation(args[1])
	if err != nil {
		return err
	}

	if srcIsRemote && dstIsRemote {
		return output.NewUsageError("source and destination cannot both be remote")
	}
	if !srcIsRemote && !dstIsRemote {
		return output.NewUsageError("either source or destination must be remote: <node-name>:<path>")
	}

	download := srcIsRemote
	var remote copyRemotePath
	var localPath string

	if download {
		remote = *srcRemote
		localPath = args[1]
	} else {
		remote = *dstRemote
		localPath = args[0]
		if err := validateLocalCopySource(localPath); err != nil {
			return err
		}
	}

	apiClient, _, err := cc.ensureAuthenticatedClient(cc.APIEndpoint)
	if err != nil {
		return err
	}

	resolvedSimulation, err := simulation.Resolve(apiClient, cc.SimulationName)
	if err != nil {
		return err
	}
	if resolvedSimulation.AutoSelected {
		_ = simulation.WriteDefaultSelectionNotice(cc.errWriter(), resolvedSimulation.Name)
	}
	simulationID := resolvedSimulation.ID

	sshHost, sshPort, err := cc.findSSHService(apiClient, simulationID)
	if err != nil {
		return err
	}

	targetNode, targetMgmtIP, err := cc.findNodeByName(apiClient, simulationID, remote.NodeName)
	if err != nil {
		return err
	}

	credentials := cc.resolveCredentials(*targetNode)

	keyPath, err := defaultKeyPathFn()
	if err != nil {
		return fmt.Errorf("failed to get SSH key path: %w", err)
	}

	timeout := cc.Timeout
	if timeout <= 0 {
		timeout = defaultCopyTimeout
	}

	copyCfg := sshpkg.BastionCopyConfig{
		BastionUser:  constant.DefaultUbuntuUser,
		BastionAddr:  net.JoinHostPort(sshHost, strconv.Itoa(sshPort)),
		BastionKey:   keyPath,
		TargetUser:   credentials.TargetUser,
		TargetAddr:   net.JoinHostPort(targetMgmtIP, "22"),
		TargetPass:   credentials.TargetPass,
		Timeout:      timeout,
		DirectTarget: credentials.DirectBastion,
	}

	if download {
		return copyFileFromBastionFn(copyCfg, remote.Path, localPath)
	}

	return copyFileViaBastionFn(copyCfg, localPath, remote.Path)
}

func parseCopyLocation(raw string) (*copyRemotePath, bool, error) {
	idx := strings.Index(raw, ":")
	if idx < 0 {
		return nil, false, nil
	}

	if idx == 0 {
		return nil, false, output.NewUsageError(fmt.Sprintf("invalid remote path: %s (expected <node-name>:<path>)", raw))
	}

	nodeName := strings.TrimSpace(raw[:idx])
	if strings.ContainsAny(nodeName, `/\`) {
		return nil, false, nil
	}

	path := strings.TrimSpace(raw[idx+1:])
	if path == "" {
		return nil, false, output.NewUsageError(fmt.Sprintf("invalid remote path: %s (expected <node-name>:<path>)", raw))
	}

	return &copyRemotePath{NodeName: nodeName, Path: path}, true, nil
}

func validateLocalCopySource(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return output.NewFileError(fmt.Sprintf("local source is not accessible: %s", path), err)
	}
	if info.IsDir() {
		return output.NewValidationError(fmt.Sprintf("copying directories is not supported: %s", path))
	}
	return nil
}

func (cc *Command) configureVerbose() {
	if cc.Verbose {
		logging.SetVerbose(os.Stderr)
		logging.Verbose("Verbose mode enabled")
	}
}

func (cc *Command) ensureAuthenticatedClient(apiEndpoint string) (*api.Client, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil || cfg.APIToken == "" {
		return nil, nil, fmt.Errorf("not authenticated. Please run 'nvair login' first")
	}

	endpoint := config.ResolveAPIEndpoint(cfg, apiEndpoint)
	return api.NewClient(endpoint, cfg.APIToken), cfg, nil
}

func (cc *Command) errWriter() io.Writer {
	if cc.Stderr != nil {
		return cc.Stderr
	}
	return io.Discard
}

func (cc *Command) findSSHService(apiClient *api.Client, simulationID string) (string, int, error) {
	services, err := apiClient.GetServices(simulationID)
	if err != nil {
		return "", 0, err
	}

	for _, service := range services {
		if (service.NodeName == constant.OOBMgmtServerName || forwardutil.IsBastionSSHServiceName(service.Name)) &&
			strings.EqualFold(service.ServiceType, "ssh") &&
			service.Host != "" &&
			service.SrcPort > 0 {
			return service.Host, service.SrcPort, nil
		}
	}

	return "", 0, fmt.Errorf("SSH service not found, run nvair create first")
}

func (cc *Command) findNodeByName(apiClient *api.Client, simulationID, nodeName string) (*api.Node, string, error) {
	nodes, err := apiClient.GetNodes(simulationID)
	if err != nil {
		return nil, "", err
	}

	images, err := apiClient.GetImages()
	if err != nil {
		return nil, "", err
	}
	imageNames := make(map[string]string, len(images))
	for _, image := range images {
		imageNames[image.ID] = image.Name
	}

	availableNodeNames := make([]string, 0, len(nodes))
	for _, n := range nodes {
		availableNodeNames = append(availableNodeNames, n.Name)
	}
	sort.Strings(availableNodeNames)

	for _, n := range nodes {
		if n.Name != nodeName {
			continue
		}

		mgmtIP, err := node.ResolveMgmtIP(n)
		if err != nil {
			return nil, "", fmt.Errorf("failed to resolve management IP for node %s: %w", n.Name, err)
		}
		if mgmtIP == "" {
			return nil, "", fmt.Errorf("node %s does not have a management IP", n.Name)
		}

		imageID := node.ResolveImageID(n)
		n.Image = imageID
		n.OS = imageID
		if resolvedImageName, ok := imageNames[imageID]; ok {
			n.OSName = resolvedImageName
		} else {
			n.OSName = imageID
		}

		return &n, mgmtIP, nil
	}

	return nil, "", output.NewNotFoundError(
		fmt.Sprintf("node not found: %s (available: %s)", nodeName, strings.Join(availableNodeNames, ", ")),
	)
}

func (cc *Command) resolveCredentials(targetNode api.Node) resolvedCredentials {
	if targetNode.Name == constant.OOBMgmtServerName {
		return resolvedCredentials{
			TargetUser:    constant.DefaultUbuntuUser,
			DirectBastion: true,
		}
	}

	imageName := strings.ToLower(strings.TrimSpace(targetNode.OSName))
	if imageName == "" {
		imageName = strings.ToLower(strings.TrimSpace(targetNode.OS))
	}
	if strings.Contains(imageName, "cumulus") {
		return resolvedCredentials{
			TargetUser: constant.DefaultCumulusUser,
			TargetPass: constant.DefaultCumulusNewPassword,
		}
	}

	return resolvedCredentials{
		TargetUser: constant.DefaultUbuntuUser,
		TargetPass: constant.DefaultUbuntuPassword,
	}
}
