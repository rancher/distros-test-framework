package qainfra

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

func (*Provisioner) provisionInfrastructure(cfg *driver.InfraConfig) (*driver.Cluster, error) {
	resources.LogLevel("info", "Start provisioning with qainfra infrastructure for %s", cfg.Product)

	// Add qainfra env config to start the provisioning process.
	infraCfg := addQAInfraEnv(cfg)

	pipeline := []ProvisioningStep{
		setupDirectories,
		prepareTerraformFiles,
		executeInfraProvisioner,
		buildClusterConfig,
		setupAnsibleEnvironment,
		executeAnsiblePlaybook,
		addTofuOutputsToConfig,
	}

	for i, step := range pipeline {
		if err := step(infraCfg); err != nil {
			return nil, fmt.Errorf("provisioning step %d failed: %w", i+1, err)
		}
	}

	resources.LogLevel("info", "Infrastructure provisioned successfully with qainfra remote"+
		" module and Ansible playbooks downloaded to: %s", infraCfg.InfraProvisioner.TempDir)

	return infraCfg.Cluster, nil
}

func (*Provisioner) destroyInfrastructure(product, module string) (string, error) {
	resources.LogLevel("info", "Start destroying qainfra infrastructure for %s with module %s",
		product, module)

	workspace := os.Getenv("TF_WORKSPACE")
	if workspace == "" {
		resources.LogLevel("warn", "No workspace specified for qainfra destroy")
		return "", errors.New("no workspace specified for qainfra destroy")
	}

	var rootDir, nodeSource string
	if resources.IsRunningInContainer() {
		resources.LogLevel("info", "Detected container environment for qainfra destroy")
		nodeSource = "/tmp/qainfra-tofu-" + workspace
	} else {
		_, callerFilePath, _, _ := runtime.Caller(0)
		rootDir = filepath.Join(filepath.Dir(callerFilePath), "..")
		nodeSource = os.Getenv("TERRAFORM_NODE_SOURCE")
		if nodeSource == "" {
			nodeSource = filepath.Join(rootDir, "tmp", "qainfra-tofu-"+workspace)
		}
	}

	if err := runCmdWithTimeout(nodeSource, 5*time.Minute,
		"tofu", "destroy", "-auto-approve", "-var-file=vars.tfvars"); err != nil {
		return "", fmt.Errorf("tofu destroy failed: %w", err)
	}

	resources.LogLevel("info", "Infrastructure destroyed successfully")

	return "cluster destroyed", nil
}
