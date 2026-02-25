package create

import (
	"fmt"
	"strings"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
)

func findOOBMgmtServer(nodes []api.Node) (string, error) {
	for _, node := range nodes {
		if node.Name == "oob-mgmt-server" {
			logging.Verbose("Found oob-mgmt-server with ID: %s", node.ID)
			return node.ID, nil
		}
	}

	logging.Verbose("oob-mgmt-server node not found")
	return "", fmt.Errorf("oob-mgmt-server node not found in simulation")
}

func findOutboundInterface(interfaces []api.Interface) (string, error) {
	for _, intf := range interfaces {
		if intf.Outbound {
			return intf.ID, nil
		}
	}

	logging.Verbose("No outbound interface found on oob-mgmt-server")
	return "", fmt.Errorf("no outbound interface found on oob-mgmt-server node")
}

func filterCumulusSwitchNodes(nodes []api.Node) []api.Node {
	return filterNodesByImage(nodes, "cumulus")
}

func filterGenericUbuntuNodes(nodes []api.Node) []api.Node {
	return filterNodesByImage(nodes, "generic")
}

func filterNodesByImage(nodes []api.Node, imageSubstring string) []api.Node {
	var filtered []api.Node
	for _, n := range nodes {
		if strings.Contains(strings.ToLower(nodeImageName(n)), imageSubstring) {
			filtered = append(filtered, n)
		}
	}

	return filtered
}

func resolveNodeImageNames(nodes []api.Node, images []api.ImageInfo) []api.Node {
	imageNamesByID := make(map[string]string, len(images))
	for _, image := range images {
		imageNamesByID[image.ID] = image.Name
	}

	resolved := make([]api.Node, len(nodes))
	copy(resolved, nodes)

	for i := range resolved {
		resolved[i].OSName = imageNamesByID[resolved[i].OS]
		if resolved[i].OSName == "" {
			resolved[i].OSName = resolved[i].OS
		}
	}

	return resolved
}

func nodeImageName(node api.Node) string {
	if node.OSName != "" {
		return node.OSName
	}

	return node.OS
}
