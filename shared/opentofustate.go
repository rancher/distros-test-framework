package shared

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type OpenTofuState struct {
	Values struct {
		RootModule struct {
			ChildModules []struct {
				Resources []struct {
					Type   string                 `json:"type"`
					Values map[string]interface{} `json:"values"`
				} `json:"resources"`
			} `json:"child_modules"`
		} `json:"root_module"`
	} `json:"values"`
}

type OpenTofuOutputs struct {
	KubeAPIHost string
	FQDN        string
}

// getAllNodesFromState extracts all node information from OpenTofu state
func getAllNodesFromState(nodeSource string) ([]InfraNode, error) {
	// Get raw state data
	stateData, err := getOpenTofuState(nodeSource)
	if err != nil {
		return nil, fmt.Errorf("failed to get OpenTofu state: %w", err)
	}

	// Parse state with structured approach
	state, err := parseStateJSON(stateData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse state: %w", err)
	}

	LogLevel("debug", "Parsed state FROMgetOpenTofuState: %+v", state)

	// Extract nodes using provider-agnostic approach
	nodes, err := extractNodesFromState(state)
	if err != nil {
		return nil, fmt.Errorf("failed to extract nodes: %w", err)
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes found in state")
	}

	logExtractedNodes(nodes)

	return nodes, nil
}

func getOpenTofuOutputs(nodeSource string) (*OpenTofuOutputs, error) {
	kubeAPIHostCmd := exec.Command("tofu", "output", "-raw", "kube_api_host")
	kubeAPIHostCmd.Dir = nodeSource
	kubeAPIHostOutput, err := kubeAPIHostCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get kube_api_host output: %w", err)
	}

	fqdnCmd := exec.Command("tofu", "output", "-raw", "fqdn")
	fqdnCmd.Dir = nodeSource
	fqdnOutput, err := fqdnCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get fqdn output: %w", err)
	}

	return &OpenTofuOutputs{
		KubeAPIHost: strings.TrimSpace(string(kubeAPIHostOutput)),
		FQDN:        strings.TrimSpace(string(fqdnOutput)),
	}, nil
}

// getOpenTofuState executes tofu command to get state JSON
func getOpenTofuState(nodeSource string) ([]byte, error) {
	stateCmd := exec.Command("tofu", "show", "-json")
	stateCmd.Dir = nodeSource

	output, err := stateCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tofu show command failed: %w", err)
	}

	return output, nil
}

// parseStateJSON parses the raw JSON into structured format
func parseStateJSON(data []byte) (*OpenTofuState, error) {
	var state OpenTofuState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("JSON unmarshal failed: %w", err)
	}

	return &state, nil
}

// extractNodesFromState extracts nodes using provider-specific extractors
func extractNodesFromState(state *OpenTofuState) ([]InfraNode, error) {
	extractors, err := getNodeExtractors()
	if err != nil {
		return nil, fmt.Errorf("failed to get node extractors: %w", err)
	}

	var nodes []InfraNode

	for _, module := range state.Values.RootModule.ChildModules {
		for _, resource := range module.Resources {
			for _, extractor := range extractors {
				if extractor.SupportsResourceType(resource.Type) {
					node, err := extractor.ExtractNode(resource.Values)
					if err != nil {
						LogLevel("debug", "Failed to extract node from resource %s: %v", resource.Type, err)
						continue
					}
					if node != nil && isValidNode(node) {
						nodes = append(nodes, *node)
					}
				}
			}
		}
	}

	return nodes, nil
}
