package qainfra

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// executeOpenTofuOperations runs OpenTofu init, workspace, and apply operations.
func executeOpenTofuOperations(config *driver.InfraConfig) error {
	resources.LogLevel("info", "Starting OpenTofu operations in %s", config.InfraProvisioner.TFNodeSource)

	if runTimeoutErr := runCmdWithTimeout(config.InfraProvisioner.TFNodeSource, 5*time.Minute,
		"tofu", "init"); runTimeoutErr != nil {
		return fmt.Errorf("tofu init failed: %w", runTimeoutErr)
	}

	runErr := runCmdWithTimeout(config.InfraProvisioner.TFNodeSource, 2*time.Minute,
		"tofu", "workspace", "new", config.InfraProvisioner.Workspace)
	if runErr != nil {
		return fmt.Errorf("tofu workspace new failed: %w", runErr)
	}

	runErr = runCmdWithTimeout(config.InfraProvisioner.TFNodeSource, 2*time.Minute,
		"tofu", "workspace", "select", config.InfraProvisioner.Workspace)
	if runErr != nil {
		return fmt.Errorf("tofu workspace select failed: %w", runErr)
	}

	args := []string{"apply", "-auto-approve", "-var-file=" + config.InfraProvisioner.TFVarsPath}
	if runTimeoutErr := runCmdWithTimeout(config.InfraProvisioner.TFNodeSource, 15*time.Minute,
		"tofu", args...); runTimeoutErr != nil {
		return fmt.Errorf("tofu apply failed: %w", runTimeoutErr)
	}

	resources.LogLevel("debug", "Completed OpenTofu operations: init, workspace new, workspace select, apply with args: %v",
		args)

	return nil
}

func addTofuOutputsToConfig(config *driver.InfraConfig) error {
	kubeAPIHostCmd := exec.Command("tofu", "output", "-raw", "kube_api_host")
	kubeAPIHostCmd.Dir = config.InfraProvisioner.TFNodeSource
	kubeAPIHostOutput, err := kubeAPIHostCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get kube_api_host output: %w", err)
	}

	fqdnCmd := exec.Command("tofu", "output", "-raw", "fqdn")
	fqdnCmd.Dir = config.InfraProvisioner.TFNodeSource
	fqdnOutput, err := fqdnCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get fqdn output: %w", err)
	}

	config.InfraProvisioner.OpenTofuOutputs.KubeAPIHost = strings.TrimSpace(string(kubeAPIHostOutput))
	config.InfraProvisioner.OpenTofuOutputs.FQDN = strings.TrimSpace(string(fqdnOutput))

	return nil
}

// setOrAppendTFVar sets or appends a key-value pair in the tfvars file, this is needed to
// ensure that missing variables upstream are added on distros without breaking anything.
func setOrAppendTFVar(tfvarsPath, key, value string) error {
	fileData, err := os.ReadFile(tfvarsPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", tfvarsPath, err)
	}

	reg := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s*=\s*".*"\s*$`)
	line := []byte(fmt.Sprintf(`%s = %q`, key, value))

	if reg.Match(fileData) {
		fileData = reg.ReplaceAll(fileData, line)
	} else {
		if len(fileData) > 0 && !bytes.HasSuffix(fileData, []byte{'\n'}) {
			fileData = append(fileData, '\n')
		}

		fileData = append(fileData, append(line, '\n')...)
	}

	return os.WriteFile(tfvarsPath, fileData, 0o644)
}

// prepareTerraformFiles copies and updates terraform configuration files.
func prepareTerraformFiles(config *driver.InfraConfig) error {
	if err := copyTerraformFiles(config); err != nil {
		return fmt.Errorf("failed to copy terraform files: %w", err)
	}

	if err := configureTerraformFiles(config); err != nil {
		return fmt.Errorf("failed to configure terraform files: %w", err)
	}

	if files, err := os.ReadDir(config.InfraProvisioner.TFNodeSource); err == nil {
		resources.LogLevel("info", "Files in working directory %s:", config.InfraProvisioner.TFNodeSource)
		for _, file := range files {
			resources.LogLevel("info", "  - %s", file.Name())
		}
	}

	return nil
}

// copyTerraformFiles copies the base terraform files to the working directory.
func copyTerraformFiles(config *driver.InfraConfig) error {
	filesToCopy := []struct {
		src, dst string
	}{
		{
			src: filepath.Join(config.InfraProvisioner.RootDir, "infrastructure", "qainfra", "main.tf"),
			dst: config.InfraProvisioner.Terraform.MainTfPath,
		},
		{
			src: filepath.Join(config.InfraProvisioner.RootDir, "infrastructure", "qainfra", "variables.tf"),
			dst: filepath.Join(config.InfraProvisioner.TFNodeSource, "variables.tf"),
		},
		{
			src: filepath.Join(config.InfraProvisioner.RootDir, "infrastructure", "qainfra", "vars.tfvars"),
			dst: config.InfraProvisioner.Terraform.TFVarsPath,
		},
	}

	for _, file := range filesToCopy {
		if err := resources.CopyFileContents(file.src, file.dst); err != nil {
			return fmt.Errorf("failed to copy %s: %w", filepath.Base(file.src), err)
		}
	}

	return nil
}

// configureTerraformFiles updates the copied terraform files with environment-specific values.
func configureTerraformFiles(config *driver.InfraConfig) error {
	if err := updateMainTfModuleSource(config.QAInfraProvider, config.InfraProvisioner.Terraform.MainTfPath); err != nil {
		return fmt.Errorf("failed to update main.tf module source: %w", err)
	}

	if err := updateVarsFile(
		config.InfraProvisioner.Terraform.TFVarsPath,
		config.InfraProvisioner.UniqueID,
		config.Product,
		config.ResourceName,
	); err != nil {
		return fmt.Errorf("failed to update vars.tfvars: %w", err)
	}

	if err := setOrAppendTFVar(
		config.InfraProvisioner.Terraform.TFVarsPath,
		"public_ssh_key",
		config.Cluster.SSH.PubKeyPath,
	); err != nil {
		return fmt.Errorf("set public_ssh_key in tfvars: %w", err)
	}

	if err := loadQAInfraTFVars(
		config.Cluster,
		config.InfraProvisioner.AirgapSetup,
		config.InfraProvisioner.ProxySetup,
		config.InfraProvisioner.TFNodeSource,
	); err != nil {
		resources.LogLevel("warn", "Failed to load vars configuration: %v", err)
	}

	return nil
}

// updateVarsFile  updates the vars.tfvars file with unique resource names and replaces product variables.
func updateVarsFile(varsFilePath, uniqueID, product, resourceName string) error {
	content, err := os.ReadFile(varsFilePath)
	if err != nil {
		return fmt.Errorf("failed to read vars file: %w", err)
	}

	varsContent := string(content)
	re := regexp.MustCompile(`aws_hostname_prefix\s*=\s*"[^"]*"`)
	varsContent = re.ReplaceAllString(varsContent, fmt.Sprintf(`aws_hostname_prefix = "dsf-%s-%s-%s"`,
		resourceName, product, uniqueID))

	userIdRe := regexp.MustCompile(`user_id\s*=\s*"[^"]*"`)
	varsContent = userIdRe.ReplaceAllString(varsContent, fmt.Sprintf(`user_id = "dsf-%s-%s"`,
		resourceName, product))

	if err := os.WriteFile(varsFilePath, []byte(varsContent), 0o644); err != nil {
		return fmt.Errorf("failed to write vars file: %w", err)
	}

	return nil
}

// updateMainTfModuleSource updates main.tf to use the correct infrastructure module based on QA_INFRA_PROVIDER env var.
func updateMainTfModuleSource(qaInfraProvider, mainTfPath string) error {
	resources.LogLevel("info", "Using infrastructure module: %s", qaInfraProvider)

	content, readErr := os.ReadFile(mainTfPath)
	if readErr != nil {
		return fmt.Errorf("failed to read main.tf: %w", readErr)
	}

	contentStr := string(content)

	// Update module source to use correct infra module.
	// todo fix hardcoded branching after upstream PR is merged.
	placeholder := "placeholder-for-remote-module"
	fmoralBranch := "distros.test.update"
	modulePath := qaInfraProvider + "/modules/cluster_nodes"

	srcModule := fmt.Sprintf("github.com/fmoral2/qa-infra-automation//tofu/%s?ref=%s", modulePath, fmoralBranch)
	contentStr = strings.ReplaceAll(contentStr, placeholder, srcModule)

	if writeErr := os.WriteFile(mainTfPath, []byte(contentStr), 0o644); writeErr != nil {
		return fmt.Errorf("failed to write updated main.tf: %w", writeErr)
	}

	resources.LogLevel("info", "Successfully updated main.tf for %s infrastructure", qaInfraProvider)

	return nil
}
