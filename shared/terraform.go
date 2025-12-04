package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
)

func setTerraformOptions(product, module string) (*terraform.Options, string, error) {
	_, callerFilePath, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(callerFilePath), "..")

	varDir, err := filepath.Abs(dir +
		fmt.Sprintf("/config/%s.tfvars", product))
	LogLevel("info", "Using tfvars in: %v", varDir)
	if err != nil {
		return nil, "", fmt.Errorf("invalid product: %s", product)
	}

	if module == "" {
		module = product
	}

	tfDir, err := filepath.Abs(dir + "/modules/" + module)
	LogLevel("info", "Using module dir: %v", tfDir)
	if err != nil {
		return nil, "", fmt.Errorf("no module found: %s", module)
	}

	terraformOptions := &terraform.Options{
		TerraformDir: tfDir,
		VarFiles:     []string{varDir},
	}

	return terraformOptions, varDir, nil
}

func loadTFconfig(
	t *testing.T,
	c *Cluster,
	product, module,
	varDir string,
	terraformOptions *terraform.Options,
) (*Cluster, error) {
	LogLevel("info", "Loading all Terraform configurations and outputs...")
	loadTFoutput(t, terraformOptions, c, module)
	loadAws(t, varDir, c)
	loadEC2(t, varDir, c)

	if err := loadTestConfig(&c.TestConfig); err != nil {
		return nil, fmt.Errorf("error loading test config: %w", err)
	}

	loadBaseConfig(t, c, product, varDir, terraformOptions)

	if product == "rke2" {
		loadRKE2Specifics(t, c, varDir, terraformOptions)
	}

	setClusterFQDN(t, c, varDir, terraformOptions)

	LogLevel("info", "Loading Version and Channel...")
	loadVersion(t, c, varDir)
	loadChannel(t, c, varDir)

	return c, nil
}

func loadBaseConfig(t *testing.T, c *Cluster, product, varDir string, terraformOptions *terraform.Options) {
	LogLevel("info", "Loading core configuration from tfvars...")

	c.Config.Product = product
	c.NodeOS = terraform.GetVariableAsStringFromVarFile(t, varDir, "node_os")
	c.Config.Arch = terraform.GetVariableAsStringFromVarFile(t, varDir, "arch")
	c.Config.ServerFlags = terraform.GetVariableAsStringFromVarFile(t, varDir, "server_flags")
	c.Config.WorkerFlags = terraform.GetVariableAsStringFromVarFile(t, varDir, "worker_flags")
	c.Config.DataStore = terraform.GetVariableAsStringFromVarFile(t, varDir, "datastore_type")
	c.Config.InstallMode = terraform.GetVariableAsStringFromVarFile(t, varDir, "install_mode")

	if c.Config.DataStore == "external" {
		loadExternalDb(t, varDir, c, terraformOptions)
	}
}

func loadRKE2Specifics(t *testing.T, c *Cluster, varDir string, terraformOptions *terraform.Options) {
	c.Config.InstallMethod = terraform.GetVariableAsStringFromVarFile(t, varDir, "install_method")

	numWinAgentsStr, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "no_of_windows_worker_nodes")
	if err != nil {
		LogLevel("debug", "no_of_windows_worker_nodes is absent from tfvars.")
		return
	}

	// Using a new helper function for safer conversion
	numWinAgents, err := safeAtoi(numWinAgentsStr)
	if err != nil {
		LogLevel("error", "Failed to convert no_of_windows_worker_nodes to int: %v", err)
		return
	}

	c.NumWinAgents = numWinAgents

	if c.NumWinAgents > 0 {
		LogLevel("info", "Loading Windows tf outputs...")
		c.WinAgentIPs = strings.Split(terraform.Output(t, terraformOptions, "windows_worker_ips"), ",")
	}
}

func setClusterFQDN(t *testing.T, c *Cluster, varDir string, terraformOptions *terraform.Options) {
	createLB := terraform.GetVariableAsStringFromVarFile(t, varDir, "create_lb")

	if createLB == "true" {
		c.FQDN = terraform.Output(t, terraformOptions, "Route53_info")
	} else {
		if len(c.ServerIPs) > 0 {
			c.FQDN = c.ServerIPs[0]
		} else {
			LogLevel("warn", "ServerIPs is empty; FQDN cannot be set to a server IP.")
			c.FQDN = "" // Or handle error if FQDN must be set
		}
	}
}

func safeAtoi(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

// func loadTFconfig(
// 	t *testing.T,
// 	c *Cluster,
// 	product, module,
// 	varDir string,
// 	terraformOptions *terraform.Options,
// ) (*Cluster, error) {
// 	LogLevel("info", "Loading TF outputs...")
// 	loadTFoutput(t, terraformOptions, c, module)

// 	LogLevel("info", "Loading tfvars in to aws config....")
// 	loadAws(t, varDir, c)

// 	LogLevel("info", "Loading tfvars in to ec2 config....")
// 	loadEC2(t, varDir, c)

// 	err := loadTestConfig(&c.TestConfig)
// 	if err != nil {
// 		return nil, fmt.Errorf("error loading test config: %w", err)
// 	}

// 	if product == "rke2" {
// 		numWinAgents, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "no_of_windows_worker_nodes")
// 		if err != nil {
// 			LogLevel("debug", "no_of_windows_worker_nodes is absent from tfvars.")
// 		} else {
// 			c.NumWinAgents, _ = strconv.Atoi(numWinAgents)
// 			if c.NumWinAgents > 0 {
// 				LogLevel("info", "Loading Windows tf outputs...")
// 				c.WinAgentIPs = strings.Split(terraform.Output(t, terraformOptions, "windows_worker_ips"), ",")
// 			}
// 		}
// 	}

// 	LogLevel("info", "Loading other tfvars in to config....")
// 	c.NodeOS = terraform.GetVariableAsStringFromVarFile(t, varDir, "node_os")
// 	c.Config.Arch = terraform.GetVariableAsStringFromVarFile(t, varDir, "arch")
// 	c.Config.Product = product
// 	c.Config.ServerFlags = terraform.GetVariableAsStringFromVarFile(t, varDir, "server_flags")
// 	c.Config.WorkerFlags = terraform.GetVariableAsStringFromVarFile(t, varDir, "worker_flags")
// 	c.Config.DataStore = terraform.GetVariableAsStringFromVarFile(t, varDir, "datastore_type")
// 	if c.Config.DataStore == "external" {
// 		loadExternalDb(t, varDir, c, terraformOptions)
// 	}
// 	createLB := terraform.GetVariableAsStringFromVarFile(t, varDir, "create_lb")
// 	if createLB == "true" {
// 		c.FQDN = terraform.Output(t, terraformOptions, "Route53_info")
// 	} else {
// 		c.FQDN = c.ServerIPs[0]
// 	}

// 	LogLevel("info", "Loading Version and Channel...")
// 	loadVersion(t, c, varDir)
// 	loadChannel(t, c, varDir)

// 	if c.Config.Product == "rke2" {
// 		c.Config.InstallMethod = terraform.GetVariableAsStringFromVarFile(t, varDir, "install_method")
// 	}
// 	c.Config.InstallMode = terraform.GetVariableAsStringFromVarFile(t, varDir, "install_mode")

// 	return c, nil
// }

// TODO: aux functions for loading data while we dont standardize from one source of truth,
//
// this is being really messy and painful. remove after.
func loadVersion(t *testing.T, c *Cluster, varDir string) {
	// defaults first always to get from env, because both local and jenkins we update this file
	if envInstallVersion := os.Getenv("INSTALL_VERSION"); envInstallVersion != "" {
		c.Config.Version = envInstallVersion
		LogLevel("info", "Using install version from env: %s", envInstallVersion)
		return
	}

	if install := os.Getenv("install_version"); install != "" {
		c.Config.Channel = install
		LogLevel("info", "Using install version from env install_version: %s", install)
		return
	}

	tfVersion := c.Config.Product + "_version"
	if tf := terraform.GetVariableAsStringFromVarFile(t, varDir, tfVersion); tf != "" {
		c.Config.Version = tf
		LogLevel("info", "Using install version from tfvars: %s", tf)
		return
	}

	versionUp := strings.ToUpper(tfVersion)
	if env := os.Getenv(tfVersion); env != "" {
		c.Config.Channel = env
		LogLevel("info", "Using install version from env to upgrade %s: %s", versionUp, env)
		return
	}
}

func loadChannel(t *testing.T, c *Cluster, varDir string) {
	// defaults first always to get from env, because both local and jenkins we update this file
	if envInstallChannel := os.Getenv("INSTALL_CHANNEL"); envInstallChannel != "" {
		c.Config.Channel = envInstallChannel
		LogLevel("info", "Using install channel from env INSTALL_CHANNEL: %s", envInstallChannel)
		return
	}

	if install := os.Getenv("install_channel"); install != "" {
		c.Config.Channel = install
		LogLevel("info", "Using install channel from env install_channel: %s", install)
		return
	}

	tfChannel := c.Config.Product + "_channel"
	if tf := terraform.GetVariableAsStringFromVarFile(t, varDir, tfChannel); tf != "" {
		c.Config.Channel = tf
		LogLevel("info", "Using install channel from tfvars: %s", tf)
		return
	}

	channelUp := strings.ToUpper(tfChannel)
	if env := os.Getenv(channelUp); env != "" {
		c.Config.Channel = env
		LogLevel("info", "Using install channel from env to upgrade %s: %s", channelUp, env)
		return
	}
}

func loadAws(t *testing.T, varDir string, c *Cluster) {
	c.Aws.Region = terraform.GetVariableAsStringFromVarFile(t, varDir, "region")
	c.Aws.AccessKeyID = os.Getenv("AWS_ACCESS_KEY_ID")
	c.Aws.SecretAccessKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	c.Aws.Subnets = terraform.GetVariableAsStringFromVarFile(t, varDir, "subnets")
	c.Aws.AvailabilityZone = terraform.GetVariableAsStringFromVarFile(t, varDir, "availability_zone")
	c.Aws.SgId = terraform.GetVariableAsStringFromVarFile(t, varDir, "sg_id")
	c.Aws.VPCID = terraform.GetVariableAsStringFromVarFile(t, varDir, "vpc_id")
}

func loadEC2(t *testing.T, varDir string, c *Cluster) {
	c.Aws.EC2.AccessKey = terraform.GetVariableAsStringFromVarFile(t, varDir, "access_key")
	c.Aws.EC2.AwsUser = terraform.GetVariableAsStringFromVarFile(t, varDir, "aws_user")
	c.Aws.EC2.Ami = terraform.GetVariableAsStringFromVarFile(t, varDir, "aws_ami")
	c.Aws.EC2.VolumeSize = terraform.GetVariableAsStringFromVarFile(t, varDir, "volume_size")
	c.Aws.EC2.InstanceClass = terraform.GetVariableAsStringFromVarFile(t, varDir, "ec2_instance_class")
	c.Aws.EC2.KeyName = terraform.GetVariableAsStringFromVarFile(t, varDir, "key_name")
}

func loadExternalDb(t *testing.T, varDir string, c *Cluster, terraformOptions *terraform.Options) {
	c.Config.ExternalDbEndpoint = terraform.Output(t, terraformOptions, "rendered_template")
	c.Config.ExternalDb = terraform.GetVariableAsStringFromVarFile(t, varDir, "external_db")
	c.Config.ExternalDbVersion = terraform.GetVariableAsStringFromVarFile(t, varDir, "external_db_version")
	c.Config.ExternalDbGroupName = terraform.GetVariableAsStringFromVarFile(t, varDir, "db_group_name")
	c.Config.ExternalDbNodeType = terraform.GetVariableAsStringFromVarFile(t, varDir, "instance_class")
}

func loadTestConfig(tc *testConfig) error {
	// extracting test tag from environment variables.
	// should pull from jenkins or local env.
	// for now only dealing with test tag.
	argsFromJenkins := os.Getenv("TEST_ARGS")
	if argsFromJenkins != "" {
		cmdStart := strings.Index(argsFromJenkins, "-tags=")
		if cmdStart == -1 {
			LogLevel("debug", "tags value not found in test args %v", argsFromJenkins)
			return nil
		}
		testTag := strings.TrimSpace(argsFromJenkins[cmdStart+len("-tags="):])
		// take the first word after -tags=.
		testTag = strings.Split(testTag, " ")[0]
		if testTag != "" {
			tc.Tag = testTag
			LogLevel("debug", "Test tag extracted from Jenkins: %s", tc.Tag)
			return nil
		}
		LogLevel("debug", "No test tag found in Jenkins args: %v", argsFromJenkins)

		return nil
	}

	if tag := os.Getenv("TEST_TAG"); tag != "" {
		tc.Tag = tag
		LogLevel("debug", "Test tag extracted from local env: %s", tc.Tag)

		return nil
	}

	return nil
}

func loadTFoutput(t *testing.T, terraformOptions *terraform.Options, c *Cluster, module string) {
	if module == "" {
		KubeConfigFile = terraform.Output(t, terraformOptions, "kubeconfig")
	}

	if c.NumBastion > 0 {
		LogLevel("info", "Loading bastion configs....")
		c.BastionConfig.PublicIPv4Addr = terraform.Output(t, terraformOptions, "bastion_ip")
		c.BastionConfig.PublicDNS = terraform.Output(t, terraformOptions, "bastion_dns")
	}

	c.ServerIPs = strings.Split(terraform.Output(t, terraformOptions, "master_ips"), ",")
	if c.NumAgents > 0 {
		c.AgentIPs = strings.Split(terraform.Output(t, terraformOptions, "worker_ips"), ",")
	}
}

// Helper function to get a string variable from a Terraform file and convert it to an integer.
func getIntVar(t *testing.T, varDir, varName string) (int, error) {
	valStr := terraform.GetVariableAsStringFromVarFile(t, varDir, varName)
	if valStr == "" {
		// Assume default is 0 if variable is missing or empty, which is common in HCL/var files
		// If the variable MUST exist, change this to return an error.
		return 0, nil
	}

	valInt, err := strconv.Atoi(valStr)
	if err != nil {
		return 0, fmt.Errorf("error converting Terraform variable '%s' to int (value: '%s'): %w", varName, valStr, err)
	}

	return valInt, nil
}

func addSplitRole(t *testing.T, sp *splitRolesConfig, varDir string, numServers int) (int, error) {
	// Map of variable names to their corresponding struct fields (pointers to simplify assignment)
	varsToFetch := map[string]*int{
		"etcd_only_nodes":   &sp.EtcdOnly,
		"etcd_cp_nodes":     &sp.EtcdCP,
		"etcd_worker_nodes": &sp.EtcdWorker,
		"cp_only_nodes":     &sp.ControlPlaneOnly,
		"cp_worker_nodes":   &sp.ControlPlaneWorker,
	}

	totalNewServers := 0

	// Use the helper function to fetch and assign all integer variables
	for varName, fieldPtr := range varsToFetch {
		count, err := getIntVar(t, varDir, varName)
		if err != nil {
			return 0, err
		}
		*fieldPtr = count
		totalNewServers += count
	}

	// Fetch the role_order string variable
	roleOrder, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "role_order")
	if err != nil {
		return 0, fmt.Errorf("error getting role_order: %w", err)
	}
	sp.RoleOrder = roleOrder

	// Calculate final server count and update the struct
	numServers += totalNewServers

	sp.Enabled = true // Enable split roles since at least one role variable was successfully processed
	sp.NumServers = numServers

	return numServers, nil
}
