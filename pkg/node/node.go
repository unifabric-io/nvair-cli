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

// ResolveMgmtIP returns the node management IP from the new top-level field first,
// and falls back to legacy metadata.mgmt_ip for older API responses.
func ResolveMgmtIP(n api.Node) (string, error) {
	if mgmtIP := strings.TrimSpace(n.ManagementIP); mgmtIP != "" {
		return mgmtIP, nil
	}
	if strings.TrimSpace(n.Metadata) == "" {
		return "", nil
	}

	metadata, err := ParseNodeMetadata(n.Metadata)
	if err != nil {
		return "", fmt.Errorf("failed to resolve management IP from node metadata: %w", err)
	}

	return strings.TrimSpace(metadata.MgmtIP), nil
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
