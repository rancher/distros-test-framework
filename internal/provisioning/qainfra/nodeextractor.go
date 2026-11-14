package qainfra

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

type infraNode struct {
	name      string
	publicIP  string
	privateIP string
	ipv6      string
	role      string
}

// address returns the IP the framework must use to reach the node.
func (n infraNode) address(airgap, ipv6Only bool) string {
	switch {
	case airgap:
		return n.privateIP
	case ipv6Only:
		return n.ipv6
	default:
		return n.publicIP
	}
}

// extractNodesFromTofuOutput reads the cluster_nodes_json Tofu output and
// translates it into the []infraNode slice the rest of the framework expects.
//
// # Single source of truth.
//
// Nodes are sorted master-first for now.
func extractNodesFromTofuOutput(config *driver.InfraConfig) ([]infraNode, *clusterNodesJSON, error) {
	data, err := fetchClusterNodesJSON(config.InfraProvisioner.TFNodeSource)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch cluster_nodes_json: %w", err)
	}

	nodes := make([]infraNode, 0, len(data.Nodes))
	for _, n := range data.Nodes {
		node := infraNode{
			name:      n.Name,
			publicIP:  n.PublicIP,
			privateIP: n.PrivateIP,
			ipv6:      n.IPv6,
			role:      strings.Join(n.Roles, ","),
		}
		nodes = append(nodes, node)
		resources.LogLevel("debug", "Extracted node from cluster_nodes_json: &{Name:%s PublicIP:%s Role:%s}",
			node.name, node.publicIP, node.role)
	}

	sort.SliceStable(nodes, func(i, j int) bool {
		im := strings.Contains(nodes[i].name, "master")
		jm := strings.Contains(nodes[j].name, "master")
		if im != jm {
			return im
		}

		return nodes[i].name < nodes[j].name
	})

	for _, node := range nodes {
		resources.LogLevel("info", "Node: %s public=%s private=%s ipv6=%s - Role: %s",
			node.name, node.publicIP, node.privateIP, node.ipv6, node.role)
	}

	return nodes, data, nil
}
