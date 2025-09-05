package qainfra

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/resources"
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

// executeOpenTofuOperations runs OpenTofu init, workspace, and apply operations
func executeOpenTofuOperations(config *InfraProvisionerConfig) error {
	resources.LogLevel("info", "Provisioning infrastructure with qa-infra remote modules...")

	// Initialize OpenTofu
	if err := runCmdWithTimeout(config.NodeSource, 5*time.Minute, "tofu", "init"); err != nil {
		return fmt.Errorf("tofu init failed: %w", err)
	}

	// Create and select workspace
	_ = runCmd(config.NodeSource, "tofu", "workspace", "new", config.Workspace) // Ignore error if exists
	if err := runCmd(config.NodeSource, "tofu", "workspace", "select", config.Workspace); err != nil {
		return fmt.Errorf("tofu workspace select failed: %w", err)
	}

	// Apply configuration
	tofuArgs := buildTofuApplyArgs(config)
	if err := runCmdWithTimeout(config.NodeSource, 15*time.Minute, "tofu", tofuArgs...); err != nil {
		return fmt.Errorf("tofu apply failed: %w", err)
	}

	resources.LogLevel("debug", "Completed OpenTofu operations")

	return nil
}

// buildTofuApplyArgs builds arguments for tofu apply command
func buildTofuApplyArgs(config *InfraProvisionerConfig) []string {
	args := []string{"apply", "-auto-approve", "-var-file=" + config.TFVarsPath}

	// Add public SSH key if available
	if akf := strings.TrimSpace(os.Getenv("ACCESS_KEY_FILE")); akf != "" {
		pubSrc := akf + ".pub"
		pubDst := filepath.Join(config.NodeSource, "id_ssh.pub")
		if err := copyFile(pubSrc, pubDst); err == nil {
			args = append(args, "-var", fmt.Sprintf("public_ssh_key=%s", pubDst))
		} else {
			args = append(args, "-var", fmt.Sprintf("public_ssh_key=%s", pubSrc))
		}
	}

	return args
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, in, 0644); err != nil {
		return err
	}
	return nil
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

	resources.LogLevel("debug", "Parsed state FROMgetOpenTofuState: %+v", state)

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
						resources.LogLevel("debug", "Failed to extract node from resource %s: %v", resource.Type, err)
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

// TerraformFiles copies and updates terraform configuration files
func prepareTerraformFiles(config *InfraProvisionerConfig) error {
	// Copy main.tf
	mainTfSrc := filepath.Join(config.RootDir, "infrastructure/qa-infra/main.tf")
	if err := copyFile(mainTfSrc, config.Terraform.MainTfPath); err != nil {
		return fmt.Errorf("failed to copy main.tf: %w", err)
	}

	// Update main.tf module source
	if err := updateMainTfModuleSource(config.QAInfraModule, config.Terraform.MainTfPath); err != nil {
		return fmt.Errorf("failed to update main.tf module source: %w", err)
	}
	// Copy variables.tf
	variablesTfSrc := filepath.Join(config.RootDir, "infrastructure/qa-infra/variables.tf")
	variablesTfDst := filepath.Join(config.NodeSource, "variables.tf")
	resources.LogLevel("info", "Copying variables.tf from %s to %s", variablesTfSrc, variablesTfDst)
	if err := copyFile(variablesTfSrc, variablesTfDst); err != nil {
		return fmt.Errorf("failed to copy variables.tf: %w", err)
	}
	resources.LogLevel("info", "Successfully copied variables.tf")

	// Copy and update vars.tfvars
	tfvarsSrc := filepath.Join(config.RootDir, "infrastructure/qa-infra/vars.tfvars")
	if err := copyAndUpdateVarsFile(tfvarsSrc, config.Terraform.TFVarsPath, config.UniqueID); err != nil {
		return fmt.Errorf("failed to prepare vars file: %w", err)
	}

	// List files in the working directory for debugging
	if files, err := os.ReadDir(config.NodeSource); err == nil {
		resources.LogLevel("info", "Files in working directory %s:", config.NodeSource)
		for _, file := range files {
			resources.LogLevel("info", "  - %s", file.Name())
		}
	}

	resources.LogLevel("debug", "Prepared terraform files")
	return nil
}

// copyAndUpdateVarsFile copies and updates vars.tfvars file with unique ID
func copyAndUpdateVarsFile(srcPath, dstPath, uniqueID string) error {
	if data, err := os.ReadFile(srcPath); err == nil {
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write vars.tfvars: %w", err)
		}
	} else {
		return fmt.Errorf("failed to read vars file: %w", err)
	}

	return updateVarsFileWithUniqueID(dstPath, uniqueID)
}

// updateMainTfModuleSource updates main.tf to use the correct infrastructure module based on INFRA_MODULE env var
func updateMainTfModuleSource(qaInfraModule, mainTfPath string) error {

	resources.LogLevel("info", "Using infrastructure module: %s", qaInfraModule)

	// Read current main.tf content
	content, err := os.ReadFile(mainTfPath)
	if err != nil {
		return fmt.Errorf("failed to read main.tf: %w", err)
	}

	contentStr := string(content)

	// Update module source to use correct infra module.
	placeholder := "placeholder-for-remote-module"
	fmoralBranch := "add.vsphere"
	modulePath := qaInfraModule + "/modules/cluster_nodes"

	srcModule := fmt.Sprintf("github.com/fmoral2/qa-infra-automation//tofu/%s?ref=%s", modulePath, fmoralBranch)
	contentStr = strings.ReplaceAll(contentStr, placeholder, srcModule)

	// Write updated content back to file
	if err := os.WriteFile(mainTfPath, []byte(contentStr), 0644); err != nil {
		return fmt.Errorf("failed to write updated main.tf: %w", err)
	}

	resources.LogLevel("debug", "Successfully updated main.tf for %s infrastructure", qaInfraModule)

	return nil
}
