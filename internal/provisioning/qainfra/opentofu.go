package qainfra

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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

	// Optionally override the `nodes` topology from env vars (split_roles or
	// no_of_server_nodes / no_of_worker_nodes).
	nodesJSON, err := buildNodesTFVar()
	if err != nil {
		return fmt.Errorf("build nodes topology from env: %w", err)
	}
	if nodesJSON != "" {
		args = append(args, "-var=nodes="+nodesJSON)
		resources.LogLevel("info", "Overriding nodes topology: %s", nodesJSON)
	}

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

	fqdn := strings.TrimSpace(string(fqdnOutput))
	config.InfraProvisioner.OpenTofuOutputs.KubeAPIHost = strings.TrimSpace(string(kubeAPIHostOutput))
	config.InfraProvisioner.OpenTofuOutputs.FQDN = fqdn
	// Surface the LB/route53 hostname on the cluster so tests that need it (e.g.
	// deployrancher's --set hostname) don't get an empty value.
	config.Cluster.FQDN = fqdn

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

// setOrAppendTFVarRaw is the bool/numeric sibling of setOrAppendTFVar.
func setOrAppendTFVarRaw(tfvarsPath, key, value string) error {
	fileData, err := os.ReadFile(tfvarsPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", tfvarsPath, err)
	}

	// Match an existing assignment of any value shape (bare, quoted, list).
	reg := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s*=\s*.*$`)
	line := []byte(fmt.Sprintf(`%s = %s`, key, value))

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

	if err := setOrAppendTFVarRaw(
		config.InfraProvisioner.Terraform.TFVarsPath,
		"create_eip",
		strconv.FormatBool(strings.EqualFold(os.Getenv("CREATE_EIP"), "true")),
	); err != nil {
		return fmt.Errorf("set create_eip in tfvars: %w", err)
	}

	if err := threadRuntimeEnvIntoTFVars(config.InfraProvisioner.Terraform.TFVarsPath); err != nil {
		return err
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

// threadRuntimeEnvIntoTFVars copies AWS_AMI and SSH_USER from the shell env
// into vars.tfvars so users can switch OS/SSH user without editing the file
// — matches the override behavior the legacy path had.
func threadRuntimeEnvIntoTFVars(tfvarsPath string) error {
	if ami := os.Getenv("AWS_AMI"); ami != "" {
		if err := setOrAppendTFVar(tfvarsPath, "aws_ami", ami); err != nil {
			return fmt.Errorf("set aws_ami in tfvars: %w", err)
		}
	}
	if sshUser := os.Getenv("SSH_USER"); sshUser != "" {
		if err := setOrAppendTFVar(tfvarsPath, "aws_ssh_user", sshUser); err != nil {
			return fmt.Errorf("set aws_ssh_user in tfvars: %w", err)
		}
	}

	return externalDBIntoTFVars(tfvarsPath)
}

// envOr returns the first non-empty of the primary/fallback env vars.
func envOr(primary, fallback string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}

	return os.Getenv(fallback)
}

// externalDBIntoTFVars writes external-datastore RDS params into vars.tfvars.
func externalDBIntoTFVars(tfvarsPath string) error {
	// Only Path B (auto-provision) has an external_db module to consume these vars.
	if !usesExternalDBProvisioning() {
		return nil
	}

	// Lowercase the engine/engine-mode selectors (tofu compares them exactly); leave credentials/versions/names as-is.
	lower := func(primary, fallback string) string {
		return strings.ToLower(strings.TrimSpace(envOr(primary, fallback)))
	}
	vars := map[string]string{
		"datastore_type":             "external",
		"external_db":                lower("EXTERNAL_DB", "external_db"),
		"external_db_version":        envOr("EXTERNAL_DB_VERSION", "external_db_version"),
		"external_db_group_name":     envOr("DB_GROUP_NAME", "db_group_name"),
		"external_db_instance_class": envOr("EXTERNAL_DB_NODE_TYPE", "instance_class"),
		"external_db_username":       envOr("DB_USERNAME", "db_username"),
		"external_db_password":       envOr("DB_PASSWORD", "db_password"),
		"external_db_engine_mode":    lower("ENGINE_MODE", "engine_mode"),
	}
	for key, value := range vars {
		if value == "" {
			continue
		}
		if err := setOrAppendTFVar(tfvarsPath, key, value); err != nil {
			return fmt.Errorf("set %s in tfvars: %w", key, err)
		}
	}

	if subnets := envOr("EXTERNAL_DB_SUBNET_IDS", "external_db_subnet_ids"); subnets != "" {
		if err := setOrAppendTFVarRaw(tfvarsPath, "external_db_subnet_ids", hclStringList(subnets)); err != nil {
			return fmt.Errorf("set external_db_subnet_ids in tfvars: %w", err)
		}
	}

	return nil
}

// hclStringList turns a comma-separated string into an HCL list literal.
func hclStringList(commaSeparated string) string {
	parts := strings.Split(commaSeparated, ",")
	for i, p := range parts {
		parts[i] = fmt.Sprintf("%q", strings.TrimSpace(p))
	}

	return "[" + strings.Join(parts, ", ") + "]"
}

// updateVarsFile updates the vars.tfvars file with unique resource names and replaces product variables.
const awsHostnamePrefixMaxLen = 24

func updateVarsFile(varsFilePath, uniqueID, product, resourceName string) error {
	content, err := os.ReadFile(varsFilePath)
	if err != nil {
		return fmt.Errorf("failed to read vars file: %w", err)
	}

	prefix := fmt.Sprintf("dsf-%s-%s-%s", resourceName, product, uniqueID)
	if len(prefix) > awsHostnamePrefixMaxLen {
		return fmt.Errorf(
			"aws_hostname_prefix %q is %d chars; AWS load-balancer / target-group "+
				"names are capped at 32 and the tofu module appends up to 8 chars "+
				"(e.g. -tg-9345), so the prefix must be ≤ %d. Shorten RESOURCE_NAME "+
				"(currently %q, %d chars).",
			prefix, len(prefix), awsHostnamePrefixMaxLen, resourceName, len(resourceName))
	}

	varsContent := string(content)
	re := regexp.MustCompile(`aws_hostname_prefix\s*=\s*"[^"]*"`)
	varsContent = re.ReplaceAllString(varsContent,
		fmt.Sprintf(`aws_hostname_prefix = %q`, prefix))

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

	// Point cluster_nodes at the upstream module.
	clusterNodesSrc := fmt.Sprintf(
		"%s//tofu/%s/modules/cluster_nodes?ref=%s", qaInfraRepo, qaInfraProvider, qaInfraRef)
	contentStr = strings.ReplaceAll(contentStr, "placeholder-for-remote-module", clusterNodesSrc)

	// Inject the external_db module only for Path B; other runs get no block, so they never fetch it.
	contentStr = strings.ReplaceAll(contentStr, externalDBMarker, externalDBModuleBlock(qaInfraProvider))

	if writeErr := os.WriteFile(mainTfPath, []byte(contentStr), 0o644); writeErr != nil {
		return fmt.Errorf("failed to write updated main.tf: %w", writeErr)
	}

	resources.LogLevel("info", "Successfully updated main.tf for %s infrastructure", qaInfraProvider)

	return nil
}

// TEMP fork/branch for pre-merge testing (tofu + ansible); revert all three to rancher/main after the PR merges.
const (
	qaInfraRepo      = "github.com/fmoral2/qa-infra-automation"
	qaInfraCloneURL  = "https://github.com/fmoral2/qa-infra-automation.git"
	qaInfraRef       = "external-datastore-support"
	externalDBMarker = "# __EXTERNAL_DB_MODULE__"
)

// usesExternalDBProvisioning reports whether to auto-provision the RDS (Path B): external + no endpoint via env or flags.
func usesExternalDBProvisioning() bool {
	if !strings.EqualFold(envOr("DATASTORE_TYPE", "datastore_type"), "external") {
		return false
	}
	if envOr("EXTERNAL_DB_ENDPOINT", "rendered_template") != "" {
		return false
	}
	if strings.Contains(strings.ToLower(envOr("SERVER_FLAGS", "server_flags")), "datastore-endpoint") {
		return false
	}

	return true
}

// externalDBModuleBlock returns the HCL for the external_db module + its output,
// or "" when auto-provisioning isn't needed (so the marker is simply removed).
func externalDBModuleBlock(qaInfraProvider string) string {
	if !usesExternalDBProvisioning() {
		return ""
	}

	src := fmt.Sprintf(
		"%s//tofu/%s/modules/external_db?ref=%s", qaInfraRepo, qaInfraProvider, qaInfraRef)

	return fmt.Sprintf(`module "external_db" {
  source              = %q
  aws_access_key      = var.aws_access_key
  aws_secret_key      = var.aws_secret_key
  aws_region          = var.aws_region
  resource_name       = var.aws_hostname_prefix
  availability_zone   = ""
  aws_security_group  = var.aws_security_group
  aws_vpc             = var.aws_vpc
  db_subnet_ids       = var.external_db_subnet_ids
  datastore_type      = var.datastore_type
  external_db         = var.external_db
  external_db_version = var.external_db_version
  db_group_name       = var.external_db_group_name
  instance_class      = var.external_db_instance_class
  db_username         = var.external_db_username
  db_password         = var.external_db_password
  engine_mode         = var.external_db_engine_mode
}

output "datastore_endpoint" {
  value     = module.external_db.datastore_endpoint
  sensitive = true
}`, src)
}
