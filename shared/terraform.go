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

	// checking if module is empty, use the product as module
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
	LogLevel("info", "Loading TF outputs...")
	loadTFoutput(t, terraformOptions, c, module)

	LogLevel("info", "Loading tfvars in to aws config....")
	loadAws(t, varDir, c)

	LogLevel("info", "Loading tfvars in to ec2 config....")
	loadEC2(t, varDir, c)

	err := loadTestConfig(&c.TestConfig)
	if err != nil {
		return nil, fmt.Errorf("error loading test config: %w", err)
	}

	if product == "rke2" {
		numWinAgents, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "no_of_windows_worker_nodes")
		if err != nil {
			LogLevel("debug", "no_of_windows_worker_nodes is absent from tfvars.")
		} else {
			c.NumWinAgents, _ = strconv.Atoi(numWinAgents)
			if c.NumWinAgents > 0 {
				LogLevel("info", "Loading Windows tf outputs...")
				c.WinAgentIPs = strings.Split(terraform.Output(t, terraformOptions, "windows_worker_ips"), ",")
			}
		}
	}

	LogLevel("info", "Loading other tfvars in to config....")
	c.NodeOS = terraform.GetVariableAsStringFromVarFile(t, varDir, "node_os")
	c.Config.Arch = terraform.GetVariableAsStringFromVarFile(t, varDir, "arch")
	c.Config.Product = product
	c.Config.ServerFlags = terraform.GetVariableAsStringFromVarFile(t, varDir, "server_flags")
	c.Config.WorkerFlags = terraform.GetVariableAsStringFromVarFile(t, varDir, "worker_flags")
	c.Config.DataStore = terraform.GetVariableAsStringFromVarFile(t, varDir, "datastore_type")
	if c.Config.DataStore == "external" {
		loadExternalDb(t, varDir, c, terraformOptions)
	}

	LogLevel("info", "Loading Version and Channel...")
	loadVersion(t, c, varDir)
	loadChannel(t, c, varDir)

	if c.Config.Product == "rke2" {
		c.Config.InstallMethod = terraform.GetVariableAsStringFromVarFile(t, varDir, "install_method")
	}
	c.Config.InstallMode = terraform.GetVariableAsStringFromVarFile(t, varDir, "install_mode")

	return c, nil
}

// TODO: aux functions for loading data while we dont standardize from one source of truth,
//
// this is being really messy and painful. remove after.
func loadVersion(t *testing.T, c *Cluster, varDir string) {
	// defaults first always to get from env, because both local and jenkins we update this file
	if install := os.Getenv("INSTALL_VERSION"); install != "" {
		c.Config.Version = install
		LogLevel("info", "Using install version from env: %s", install)
		return
	}

	version := c.Config.Product + "_version"
	if tf := terraform.GetVariableAsStringFromVarFile(t, varDir, version); tf != "" {
		c.Config.Version = tf
		LogLevel("info", "Using install version from tfvars: %s", tf)
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

	tfChannel := c.Config.Product + "_channel"

	if tf := terraform.GetVariableAsStringFromVarFile(t, varDir, tfChannel); tf != "" {
		c.Config.Channel = tf
		LogLevel("info", "Using install channel from tfvars: %s", tf)
		return
	}

	if install := os.Getenv("install_channel"); install != "" {
		c.Config.Channel = install
		LogLevel("info", "Using install channel from env install_channel: %s", install)
		return
	}

	channelUp := strings.ToUpper(tfChannel)
	if env := os.Getenv(channelUp); env != "" {
		c.Config.Channel = env
		LogLevel("info", "Using install channel from env %s: %s", channelUp, env)
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
		c.FQDN = terraform.Output(t, terraformOptions, "Route53_info")
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

func addSplitRole(t *testing.T, sp *splitRolesConfig, varDir string, numServers int) (int, error) {
	etcdNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"etcd_only_nodes",
	))
	if err != nil {
		return 0, fmt.Errorf("error getting etcd_only_nodes %w", err)
	}
	etcdCpNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"etcd_cp_nodes",
	))
	if err != nil {
		return 0, fmt.Errorf("error getting etcd_cp_nodes %w", err)
	}
	etcdWorkerNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"etcd_worker_nodes",
	))
	if err != nil {
		return 0, fmt.Errorf("error getting etcd_worker_nodes %w", err)
	}
	cpNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"cp_only_nodes",
	))
	if err != nil {
		return 0, fmt.Errorf("error getting cp_only_nodes %w", err)
	}
	cpWorkerNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"cp_worker_nodes",
	))
	if err != nil {
		return 0, fmt.Errorf("error getting cp_worker_nodes %w", err)
	}

	numServers = numServers + etcdNodes + etcdCpNodes + etcdWorkerNodes + cpNodes + cpWorkerNodes

	sp.Add = true
	sp.ControlPlaneOnly = cpNodes
	sp.EtcdOnly = etcdNodes
	sp.EtcdCP = etcdCpNodes
	sp.EtcdWorker = etcdWorkerNodes
	sp.ControlPlaneWorker = cpWorkerNodes
	sp.NumServers = numServers

	return numServers, nil
}
