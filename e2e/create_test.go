//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/topology"
	"gopkg.in/yaml.v3"
)

// TestIntegration_RealAPI_Create exercises the real NVIDIA Air API using nvair.
// It requires NV_AIR_USER and NV_AIR_TOKEN to be set and skips otherwise.
func TestIntegration_RealAPI_Create(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	user := os.Getenv("NV_AIR_USER")
	token := os.Getenv("NV_AIR_TOKEN")
	if user == "" || token == "" {
		t.Skip("NV_AIR_USER and NV_AIR_TOKEN must be set for real API e2e test")
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	env := withHomeEnv(homeDir)

	loginResult := runCommand(t, env, "login", "-u", user, "-p", token)
	logCommandOutput(t, loginResult, "TestIntegration_RealAPI_Create_login")
	if loginResult.ExitCode != 0 {
		t.Fatalf("nvair login failed with exit code %d: %v", loginResult.ExitCode, loginResult.Err)
	}
	if !strings.Contains(loginResult.Stdout+loginResult.Stderr, "Login successful") {
		t.Fatalf("unexpected login output: stdout=%q stderr=%q", loginResult.Stdout, loginResult.Stderr)
	}

	configPath, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("could not determine config path: %v", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file not created after login: %v", err)
	}

	simName := "simple"
	bootstrapNode := "node-gpu-1"
	topoDir := filepath.Join("..", "examples", "simple")
	installScriptPath := filepath.Join(topoDir, "install.sh")
	assertUnifiedInstallScriptExists(t, installScriptPath)
	externalKubeconfigPath := filepath.Join(homeDir, "kubeconfig-simple-external.yaml")

	t.Logf("Installing simulation `%s` using `%s`", simName, installScriptPath)

	t.Cleanup(func() {
		deleteResult := runCommand(t, env, "delete", "simulation", simName)
		logCommandOutput(t, deleteResult, "TestIntegration_RealAPI_Create_cleanup_delete")
	})

	installResult := runBashScript(t, env, installScriptPath, "--delete-if-exists", "--bootstrap-node", bootstrapNode, "-o", externalKubeconfigPath)
	logCommandOutput(t, installResult, "TestIntegration_RealAPI_Create_install")
	if installResult.ExitCode != 0 {
		collectRemoteKubesprayLog(t, env, simName, bootstrapNode)
		t.Fatalf("install.sh failed with exit code %d: %v", installResult.ExitCode, installResult.Err)
	}
	if _, err := os.Stat(externalKubeconfigPath); err != nil {
		t.Fatalf("expected external kubeconfig at %q, got error: %v", externalKubeconfigPath, err)
	}

	getSimulationsResult := runCommand(t, env, "get", "simulations")
	logCommandOutput(t, getSimulationsResult, "TestIntegration_RealAPI_Create_get_simulations")
	if getSimulationsResult.ExitCode != 0 {
		t.Fatalf("nvair get simulations failed with exit code %d: %v", getSimulationsResult.ExitCode, getSimulationsResult.Err)
	}
	if !strings.Contains(getSimulationsResult.Stdout+getSimulationsResult.Stderr, simName) {
		t.Fatalf("expected get simulations output to include %q, stdout=%q stderr=%q", simName, getSimulationsResult.Stdout, getSimulationsResult.Stderr)
	}

	actualNodeNamesJSON := getNodeNameMapFromCLI(t, env, simName, "json")
	actualNodeNamesYAML := getNodeNameMapFromCLI(t, env, simName, "yaml")
	expectedNodeNames := loadExpectedNodeNameMapFromTopology(t, topoDir)

	assertAllExpectedNodesPresent(t, expectedNodeNames, actualNodeNamesJSON, "json")
	assertAllExpectedNodesPresent(t, expectedNodeNames, actualNodeNamesYAML, "yaml")
	assertExecIPsMatchNetplan(t, env, simName, topoDir)
	assertExecExitCode(t, env, simName, "node-gpu-1", 99)
	assertExecQuotedShellCommand(t, env, simName, "node-gpu-1")
	assertUnifiedInstallScriptExists(t, installScriptPath)
}

type cliNodeOutput struct {
	Name string `json:"name" yaml:"name"`
}

func getNodeNameMapFromCLI(t *testing.T, env []string, simulationName, outputFormat string) map[string]struct{} {
	t.Helper()

	result := runCommand(t, env, "get", "nodes", "--simulation", simulationName, "-o", outputFormat)
	logCommandOutput(t, result, "TestIntegration_RealAPI_Create_get_nodes_"+outputFormat)
	if result.ExitCode != 0 {
		t.Fatalf("nvair get nodes -o %s failed with exit code %d: %v", outputFormat, result.ExitCode, result.Err)
	}

	nodes := parseCLINodeOutput(t, result.Stdout, outputFormat)
	nameSet := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		name := strings.TrimSpace(node.Name)
		if name != "" {
			nameSet[name] = struct{}{}
		}
	}

	return nameSet
}

func parseCLINodeOutput(t *testing.T, stdout, outputFormat string) []cliNodeOutput {
	t.Helper()

	var nodes []cliNodeOutput
	var err error

	switch outputFormat {
	case "json":
		err = json.Unmarshal([]byte(stdout), &nodes)
	case "yaml":
		err = yaml.Unmarshal([]byte(stdout), &nodes)
	default:
		t.Fatalf("unsupported output format: %s", outputFormat)
	}

	if err != nil {
		t.Fatalf("failed to parse get nodes -o %s output: %v, stdout=%q", outputFormat, err, stdout)
	}

	return nodes
}

func loadExpectedNodeNameMapFromTopology(t *testing.T, directory string) map[string]struct{} {
	t.Helper()

	rawTopology, err := topology.LoadTopologyFromDirectory(directory)
	if err != nil {
		t.Fatalf("failed to load topology from %q: %v", directory, err)
	}

	expected := make(map[string]struct{}, len(rawTopology.Content.Nodes))
	for nodeKey, nodeValue := range rawTopology.Content.Nodes {
		nodeName := strings.TrimSpace(nodeKey)

		if nodeMap, ok := nodeValue.(map[string]interface{}); ok {
			if valueName, ok := nodeMap["name"].(string); ok && strings.TrimSpace(valueName) != "" {
				nodeName = strings.TrimSpace(valueName)
			}
		}

		if nodeName != "" {
			expected[nodeName] = struct{}{}
		}
	}

	if len(expected) == 0 {
		t.Fatalf("topology at %q contains no nodes", directory)
	}

	return expected
}

func assertAllExpectedNodesPresent(t *testing.T, expectedNodeNameMap, actualNodeNameMap map[string]struct{}, outputFormat string) {
	t.Helper()

	for expectedNodeName := range expectedNodeNameMap {
		if _, exists := actualNodeNameMap[expectedNodeName]; !exists {
			t.Fatalf("get nodes -o %s missing expected node %q; actual nodes: %v", outputFormat, expectedNodeName, mapKeys(actualNodeNameMap))
		}
	}
}

type ipAddrInterface struct {
	IfName   string `json:"ifname"`
	AddrInfo []struct {
		Family string `json:"family"`
		Local  string `json:"local"`
	} `json:"addr_info"`
}

type nodeNetplan struct {
	Network struct {
		Ethernets map[string]struct {
			Addresses []string `yaml:"addresses"`
		} `yaml:"ethernets"`
	} `yaml:"network"`
}

func assertExecIPsMatchNetplan(t *testing.T, env []string, simulationName, topologyDir string) {
	t.Helper()

	nodes := []string{"node-gpu-1", "node-gpu-2", "node-gpu-3", "node-gpu-4"}
	for _, nodeName := range nodes {
		netplanPath := filepath.Join(topologyDir, nodeName+".yaml")
		expectedIPs := loadExpectedInterfaceIPsFromNetplan(t, netplanPath)

		result := runCommand(t, env, "exec", nodeName, "-s", simulationName, "-v", "--", "ip", "-j", "addr")
		logCommandOutput(t, result, "TestIntegration_RealAPI_Create_exec_ip_j_addr_"+nodeName)
		if result.ExitCode != 0 {
			t.Fatalf("nvair exec %s failed with exit code %d: stdout=%q stderr=%q", nodeName, result.ExitCode, result.Stdout, result.Stderr)
		}

		actualIPs := parseIPAddrInterfaceMap(t, result.Stdout, nodeName)

		for iface, expectedIP := range expectedIPs {
			actualIP, ok := actualIPs[iface]
			if !ok {
				t.Fatalf("node %s missing interface %s in ip -j addr output; available interfaces: %v", nodeName, iface, mapStringKeys(actualIPs))
			}
			if actualIP != expectedIP {
				t.Fatalf("node %s interface %s IP mismatch: expected %s, got %s", nodeName, iface, expectedIP, actualIP)
			}
		}
	}
}

func assertExecExitCode(t *testing.T, env []string, simulationName, nodeName string, expectedExitCode int) {
	t.Helper()

	result := runCommand(
		t,
		env,
		"exec",
		nodeName,
		"-s",
		simulationName,
		"--",
		"sh",
		"-c",
		fmt.Sprintf("exit %d", expectedExitCode),
	)
	logCommandOutput(t, result, "TestIntegration_RealAPI_Create_exec_exit_code_"+nodeName)

	if result.ExitCode != expectedExitCode {
		t.Fatalf(
			"nvair exec %s expected exit code %d, got %d: stdout=%q stderr=%q",
			nodeName,
			expectedExitCode,
			result.ExitCode,
			result.Stdout,
			result.Stderr,
		)
	}
}

func assertExecQuotedShellCommand(t *testing.T, env []string, simulationName, nodeName string) {
	t.Helper()

	shellCommand := "echo 11 ; echo 456"
	result := runCommand(
		t,
		env,
		"exec",
		nodeName,
		"-s",
		simulationName,
		"--",
		"sh",
		"-c",
		shellCommand,
	)
	logCommandOutput(t, result, "TestIntegration_RealAPI_Create_exec_quoted_shell_"+nodeName)

	if result.ExitCode != 0 {
		t.Fatalf(
			"nvair exec %s with quoted shell command %q failed: exit=%d stdout=%q stderr=%q",
			nodeName,
			shellCommand,
			result.ExitCode,
			result.Stdout,
			result.Stderr,
		)
	}

	lines := nonEmptyTrimmedLines(result.Stdout)
	expectedLines := []string{"11", "456"}
	if !hasOrderedSubsequence(lines, expectedLines) {
		t.Fatalf(
			"unexpected output for quoted shell command %q on %s: expected ordered lines %v, got %v (raw stdout=%q)",
			shellCommand,
			nodeName,
			expectedLines,
			lines,
			result.Stdout,
		)
	}
}

func assertUnifiedInstallScriptExists(t *testing.T, installScriptPath string) {
	t.Helper()
	if _, err := os.Stat(installScriptPath); err != nil {
		t.Fatalf("unified install script not found at %q: %v", installScriptPath, err)
	}
}

func collectRemoteKubesprayLog(t *testing.T, env []string, simulationName, bootstrapNode string) {
	t.Helper()

	artifactDir := filepath.Join(os.TempDir(), "nvair-e2e")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Logf("failed to create Kubespray log artifact directory %q: %v", artifactDir, err)
		return
	}

	localLogPath := filepath.Join(
		artifactDir,
		fmt.Sprintf("%s-%s-kubespray.log", safeArtifactName(t.Name()), safeArtifactName(simulationName)),
	)
	remoteLogPath := fmt.Sprintf("%s:/tmp/kubespray.log", bootstrapNode)

	result := runCommand(t, env, "--verbose", "cp", "-s", simulationName, remoteLogPath, localLogPath)
	logCommandOutput(t, result, "TestIntegration_RealAPI_Create_collect_kubespray_log")
	if result.ExitCode != 0 {
		t.Logf("failed to copy remote Kubespray log %s to %s; install.sh may not have created /tmp/kubespray.log", remoteLogPath, localLogPath)
		return
	}

	content, err := os.ReadFile(localLogPath)
	if err != nil {
		t.Logf("copied remote Kubespray log to %s but failed to read it: %v", localLogPath, err)
		return
	}

	t.Logf("copied remote Kubespray log %s to local artifact %s", remoteLogPath, localLogPath)
	t.Logf("--- kubespray.log tail ---\n%s", tailLines(string(content), 200))
}

func safeArtifactName(raw string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		" ", "_",
	)
	return replacer.Replace(raw)
}

func tailLines(content string, maxLines int) string {
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return "(empty)"
	}

	lines := strings.Split(content, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		return strings.Join(lines[len(lines)-maxLines:], "\n")
	}
	return strings.Join(lines, "\n")
}

func runBashScript(t *testing.T, env []string, scriptPath string, args ...string) *CommandResult {
	t.Helper()

	if env == nil {
		env = os.Environ()
	}

	cmdArgs := make([]string, 0, 1+len(args))
	cmdArgs = append(cmdArgs, scriptPath)
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command("bash", cmdArgs...)

	cmdEnv := make([]string, 0, len(env)+1)
	cmdEnv = append(cmdEnv, env...)
	cmdEnv = append(cmdEnv, "NVAIR_BIN="+getCliBinaryPath(t))
	cmd.Env = cmdEnv

	var stdout, stderr bytes.Buffer
	stdoutWriter := io.Writer(&stdout)
	stderrWriter := io.Writer(&stderr)
	if testing.Verbose() {
		stdoutWriter = io.MultiWriter(&stdout, os.Stdout)
		stderrWriter = io.MultiWriter(&stderr, os.Stderr)
	}
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	return &CommandResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Err:      err,
	}
}

func loadExpectedInterfaceIPsFromNetplan(t *testing.T, netplanPath string) map[string]string {
	t.Helper()

	raw, err := os.ReadFile(netplanPath)
	if err != nil {
		t.Fatalf("failed to read netplan file %q: %v", netplanPath, err)
	}

	var cfg nodeNetplan
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("failed to parse netplan file %q: %v", netplanPath, err)
	}

	expected := make(map[string]string, len(cfg.Network.Ethernets))
	for iface, details := range cfg.Network.Ethernets {
		for _, address := range details.Addresses {
			if ip := extractIP(address); ip != "" {
				expected[iface] = ip
				break
			}
		}
	}

	if len(expected) == 0 {
		t.Fatalf("no interface addresses found in netplan %q", netplanPath)
	}

	return expected
}

func extractIP(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}

	if ip, _, err := net.ParseCIDR(address); err == nil {
		return ip.String()
	}

	parsed := net.ParseIP(address)
	if parsed == nil {
		return ""
	}
	return parsed.String()
}

func parseIPAddrInterfaceMap(t *testing.T, stdout, nodeName string) map[string]string {
	t.Helper()

	var interfaces []ipAddrInterface
	if err := json.Unmarshal([]byte(stdout), &interfaces); err != nil {
		t.Fatalf("failed to parse ip -j addr output for %s: %v, stdout=%q", nodeName, err, stdout)
	}

	actual := make(map[string]string, len(interfaces))
	for _, iface := range interfaces {
		for _, addrInfo := range iface.AddrInfo {
			if addrInfo.Family != "inet" {
				continue
			}
			ip := strings.TrimSpace(addrInfo.Local)
			if ip != "" {
				actual[iface.IfName] = ip
				break
			}
		}
	}

	return actual
}

func mapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func mapStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func nonEmptyTrimmedLines(output string) []string {
	rawLines := strings.Split(output, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, rawLine := range rawLines {
		line := strings.TrimSpace(rawLine)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func hasOrderedSubsequence(lines, expected []string) bool {
	if len(expected) == 0 {
		return true
	}

	needleIndex := 0
	for _, line := range lines {
		if line == expected[needleIndex] {
			needleIndex++
			if needleIndex == len(expected) {
				return true
			}
		}
	}
	return false
}

func withHomeEnv(home string) []string {
	env := []string{"HOME=" + home}
	env = append(env, os.Environ()...)
	return env
}

func writeE2ETestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
