package node

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/unifabric-io/nvair-cli/pkg/api"
)

// NodeMetadata holds parsed metadata from a node
type NodeMetadata struct {
	MgmtIP string `json:"mgmt_ip" yaml:"mgmtIP"`
}

// ParseNodeMetadata parses the metadata JSON from a node
func ParseNodeMetadata(metadata string) (*NodeMetadata, error) {
	var nm NodeMetadata
	if err := json.Unmarshal([]byte(metadata), &nm); err != nil {
		return nil, fmt.Errorf("failed to parse node metadata: %w", err)
	}
	return &nm, nil
}

// ResolveMgmtIP returns the node management IP. It checks, in order:
//  1. the top-level "management_ip" field (older API responses),
//  2. the "management_interfaces" map (current /v3/simulations/nodes/
//     responses, which expose per-interface entries like
//     {"eth0": {"ip": "...", "mac_address": "..."}} instead of a single
//     top-level management IP).
func ResolveMgmtIP(n api.Node) (string, error) {
	if mgmtIP := strings.TrimSpace(n.ManagementIP); mgmtIP != "" {
		return mgmtIP, nil
	}

	return resolveMgmtIPFromInterfaces(n.ManagementInterfaces), nil
}

// resolveMgmtIPFromInterfaces returns the first non-empty IP found in the
// management interfaces map, iterating interface names in sorted order for
// deterministic results (nodes commonly expose a single "eth0"/"eth1" entry).
func resolveMgmtIPFromInterfaces(interfaces map[string]api.ManagementInterface) string {
	if len(interfaces) == 0 {
		return ""
	}

	names := make([]string, 0, len(interfaces))
	for name := range interfaces {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if ip := strings.TrimSpace(interfaces[name].IP); ip != "" {
			return ip
		}
	}
	return ""
}

// ResolveImageID returns the node image identifier from the new top-level image
// field first, and falls back to the legacy os field for older API responses.
func ResolveImageID(n api.Node) string {
	if imageID := strings.TrimSpace(n.Image); imageID != "" {
		return imageID
	}
	return strings.TrimSpace(n.OS)
}

// SortNodesByName sorts nodes by their name in ascending order
func SortNodesByName(nodes []api.Node) {
	sort.Slice(nodes, func(i, j int) bool {
		// Extract numeric part from node names for proper sorting
		// e.g., node-1 < node-2 < node-10
		return extractNodeNumber(nodes[i].Name) < extractNodeNumber(nodes[j].Name)
	})
}

// extractNodeNumber extracts the numeric suffix from a node name
// e.g., "node-1" -> 1, "node-gpu-2" -> 2
func extractNodeNumber(name string) int {
	parts := strings.Split(name, "-")
	for i := len(parts) - 1; i >= 0; i-- {
		if num := parseNumber(parts[i]); num >= 0 {
			return num
		}
	}
	return 0
}

// parseNumber attempts to parse a string as an integer
func parseNumber(s string) int {
	var num int
	_, err := fmt.Sscanf(s, "%d", &num)
	if err != nil {
		return -1
	}
	return num
}
