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
	MgmtIP string `json:"mgmt_ip"`
}

// ParseNodeMetadata parses the metadata JSON from a node
func ParseNodeMetadata(metadata string) (*NodeMetadata, error) {
	var nm NodeMetadata
	if err := json.Unmarshal([]byte(metadata), &nm); err != nil {
		return nil, fmt.Errorf("failed to parse node metadata: %w", err)
	}
	return &nm, nil
}

// SortNodesByName sorts nodes by their name in ascending order
func SortNodesByName(nodes []api.Node) {
	sort.Slice(nodes, func(i, j int) bool {
		// Extract numeric part from node names for proper sorting
		// e.g., node-1 < node-2 < node-10
		return extractNodeNumber(nodes[i].Name) < extractNodeNumber(nodes[j].Name)
	})
}

// FilterGPUNodes filters nodes with "node-gpu" prefix
func FilterGPUNodes(nodes []api.Node) []api.Node {
	var gpuNodes []api.Node
	for _, n := range nodes {
		if strings.HasPrefix(n.Name, "node-gpu") {
			gpuNodes = append(gpuNodes, n)
		}
	}
	return gpuNodes
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
