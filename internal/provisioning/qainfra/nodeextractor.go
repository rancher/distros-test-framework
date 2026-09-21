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
	role      string
	os        string
}

// address returns the IP the framework must use to reach the node.
func (n infraNode) address(airgap bool) string {
	if airgap {
		return n.privateIP
	}

	return n.publicIP
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
		nodeOS := n.OS
		if nodeOS == "" {
			if strings.Contains(strings.ToLower(n.Name), "windows") {
				nodeOS = "windows"
			} else {
				nodeOS = "linux"
			}
		}
		node := infraNode{
			name:      n.Name,
			publicIP:  n.PublicIP,
			privateIP: n.PrivateIP,
			role:      strings.Join(n.Roles, ","),
			os:        nodeOS,
		}
		nodes = append(nodes, node)
		resources.LogLevel("debug", "Extracted node from cluster_nodes_json: &{Name:%s PublicIP:%s Role:%s OS:%s}",
			node.name, node.publicIP, node.role, node.os)
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
		resources.LogLevel("info", "Node: %s public=%s private=%s - Role: %s",
			node.name, node.publicIP, node.privateIP, node.role)
	}

	return nodes, data, nil
}
