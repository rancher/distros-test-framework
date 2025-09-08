package qainfra

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

type infraNode struct {
	name     string
	publicIP string
	role     string
}

type newOpenTofuState struct {
	Resources []struct {
		Type      string `json:"type"`
		Instances []struct {
			Attributes struct {
				PublicIP string            `json:"public_ip"`
				Tags     map[string]string `json:"tags"`
			} `json:"attributes"`
		} `json:"instances"`
	} `json:"resources"`
}

// extractNodesFromState extracts node information from OpenTofu state.
// todo: refactor to a generic method once vsphere is supported.
func extractNodesFromTFState(config *driver.InfraConfig) ([]infraNode, error) {
	stateTfFile := filepath.Join(config.InfraProvisioner.TFNodeSource,
		"terraform.tfstate.d", config.InfraProvisioner.Workspace, "terraform.tfstate")

	stateData, err := os.ReadFile(stateTfFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state newOpenTofuState
	if err := json.Unmarshal(stateData, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state JSON: %w", err)
	}

	// extract nodes from new state format.
	var nodes []infraNode
	for _, resource := range state.Resources {
		if resource.Type == "aws_instance" {
			for _, instance := range resource.Instances {
				name := instance.Attributes.Tags["Name"]
				publicIP := instance.Attributes.PublicIP

				if name != "" && publicIP != "" {
					role := addRoleFromName(name)

					node := infraNode{
						name:     name,
						publicIP: publicIP,
						role:     role,
					}
					nodes = append(nodes, node)
					resources.LogLevel("debug", "Extracted AWS node: &{Name:%s PublicIP:%s Role:%s}",
						node.name, node.publicIP, node.role)
				}
			}
		}
	}

	for _, node := range nodes {
		resources.LogLevel("info", "Node: %s (%s) - Role: %s", node.name, &node, node.role)
	}

	return nodes, nil
}

// addRoleFromName determines the role based on the node name since QAInfra does not provide roles directly,
// we need to infer them based on given remote module naming conventions.
func addRoleFromName(name string) string {
	// remove prefix.
	name = regexp.MustCompile(`^tf-dsf-.*?-(k3s|rke2)-`).ReplaceAllString(name, "")

	// for now on remote infra the first one is referred as master.
	switch {
	case strings.Contains(name, "master"):
		return "etcd,cp,worker"
	case strings.Contains(name, "etcd-cp-worker"):
		return "etcd,cp,worker"
	case strings.Contains(name, "etcd"):
		return "etcd"
	case strings.Contains(name, "cp"):
		return "cp"
	case strings.Contains(name, "worker"):
		return "worker"
	default:
		return "master"
	}
}
