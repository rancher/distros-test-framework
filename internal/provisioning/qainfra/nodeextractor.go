package qainfra

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

	// Tofu doesn't guarantee state-iteration order, but tests like
	// cluster-reset assume ServerIPs[0] is the cluster-init / primary master
	// node (the one with `cluster-init: true`, no `server:` URL). Sort so
	// any node whose role-suffix is "master" comes first; rest go in
	// alphabetical order for deterministic ServerIPs/AgentIPs ordering
	// across runs.
	//
	// Match against the stripped suffix (not the full EC2 Name tag), so a
	// RESOURCE_NAME containing "master" doesn't accidentally tag every node
	// as a master candidate.
	sort.SliceStable(nodes, func(i, j int) bool {
		iSuf := stripNodeNamePrefix(nodes[i].name)
		jSuf := stripNodeNamePrefix(nodes[j].name)
		im := strings.Contains(iSuf, "master")
		jm := strings.Contains(jSuf, "master")
		if im != jm {
			return im
		}

		return iSuf < jSuf
	})

	for _, node := range nodes {
		resources.LogLevel("info", "Node: %s (%s) - Role: %s", node.name, &node, node.role)
	}

	return nodes, nil
}

// nodeNamePrefixRE strips the framework-generated prefix from an EC2 Name
// tag, leaving just the role-suffix. Greedy on the middle so the LAST
// `-(k3s|rke2)-` ends the prefix — RESOURCE_NAME can legitimately contain
// "k3s"/"rke2" tokens (e.g. RESOURCE_NAME=foo-k3s-master), and only the
// framework-appended trailing `-<product>-<id>-<role>` is fixed-shape.
//
//	tf-dsf-fmora4-rke2-aBc12-master                     -> aBc12-master
//	tf-dsf-master-test-k3s-aBc12-worker-0               -> aBc12-worker-0
//	tf-dsf-foo-k3s-master-rke2-aBc12-worker-0           -> aBc12-worker-0
var nodeNamePrefixRE = regexp.MustCompile(`^tf-dsf-.*-(k3s|rke2)-`)

// stripNodeNamePrefix returns the role-suffix portion of a Tofu-generated
// node name. Centralizes the regex so role-classification (addRoleFromName)
// and primary-master detection (sort comparator) can't drift apart.
func stripNodeNamePrefix(name string) string {
	return nodeNamePrefixRE.ReplaceAllString(name, "")
}

// addRoleFromName determines the role based on the node name since QAInfra does not provide roles directly,
// we need to infer them based on given remote module naming conventions.
func addRoleFromName(name string) string {
	suffix := stripNodeNamePrefix(name)

	// for now on remote infra the first one is referred as master.
	switch {
	case strings.Contains(suffix, "master"):
		return "etcd,cp,worker"
	case strings.Contains(suffix, "etcd-cp-worker"):
		return "etcd,cp,worker"
	case strings.Contains(suffix, "etcd"):
		return "etcd"
	case strings.Contains(suffix, "cp"):
		return "cp"
	case strings.Contains(suffix, "worker"):
		return "worker"
	default:
		return "master"
	}
}
