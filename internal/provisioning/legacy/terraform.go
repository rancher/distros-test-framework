package legacy

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"

	"github.com/rancher/distros-test-framework/internal/provisioning"
	"github.com/rancher/distros-test-framework/internal/resources"
)

func setTerraformOptions(product, module string) (*terraform.Options, string, error) {
	_, callerFilePath, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(callerFilePath), "..")

	varDir, err := filepath.Abs(dir +
		fmt.Sprintf("/config/%s.tfvars", product))
	resources.LogLevel("info", "Using tfvars in: %v", varDir)
	if err != nil {
		return nil, "", fmt.Errorf("invalid product: %s", product)
	}

	// checking if module is empty, use the product as module
	if module == "" {
		module = product
	}

	tfDir, err := filepath.Abs(dir + "/modules/" + module)
	resources.LogLevel("info", "Using module dir: %v", tfDir)
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
	c *provisioning.Cluster,
	product, module,
	varDir string,
	terraformOptions *terraform.Options,
) (*provisioning.Cluster, error) {
	resources.LogLevel("info", "Loading TF outputs...")
	loadTFoutput(t, terraformOptions, c, module)

	resources.LogLevel("info", "Loading tfvars in to aws config....")
	loadAws(t, varDir, c)

	resources.LogLevel("info", "Loading tfvars in to ec2 config....")
	loadEC2(t, varDir, c)

	err := loadTestConfig(&c.TestConfig)
	if err != nil {
		return nil, fmt.Errorf("error loading test config: %w", err)
	}

	if product == "rke2" {
		numWinAgents, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "no_of_windows_worker_nodes")
		if err != nil {
			resources.LogLevel("debug", "no_of_windows_worker_nodes is absent from tfvars.")
		} else {
			c.NumWinAgents, _ = strconv.Atoi(numWinAgents)
			if c.NumWinAgents > 0 {
				resources.LogLevel("info", "Loading Windows tf outputs...")
				c.WinAgentIPs = strings.Split(terraform.Output(t, terraformOptions, "windows_worker_ips"), ",")
			}
		}
	}

	resources.LogLevel("info", "Loading other tfvars in to config....")
	c.NodeOS = terraform.GetVariableAsStringFromVarFile(t, varDir, "node_os")
	c.Config.Arch = terraform.GetVariableAsStringFromVarFile(t, varDir, "arch")
	c.Config.Product = product
	c.Config.ServerFlags = terraform.GetVariableAsStringFromVarFile(t, varDir, "server_flags")
	c.Config.WorkerFlags = terraform.GetVariableAsStringFromVarFile(t, varDir, "worker_flags")
	c.Config.DataStore = terraform.GetVariableAsStringFromVarFile(t, varDir, "datastore_type")
	if c.Config.DataStore == "external" {
		loadExternalDb(t, varDir, c, terraformOptions)
	}

	resources.LogLevel("info", "Loading Version and Channel...")
	loadVersion(t, c, varDir)
	loadChannel(t, c, varDir)

	if c.Config.Product == "rke2" {
		c.Config.InstallMethod = terraform.GetVariableAsStringFromVarFile(t, varDir, "install_method")
	}
	c.Config.InstallMode = terraform.GetVariableAsStringFromVarFile(t, varDir, "install_mode")

	return c, nil
}

func loadTFoutput(t *testing.T, terraformOptions *terraform.Options, c *provisioning.Cluster, module string) {
	if module == "" {
		resources.KubeConfigFile = terraform.Output(t, terraformOptions, "kubeconfig")
		c.FQDN = terraform.Output(t, terraformOptions, "Route53_info")
	}

	if c.NumBastion > 0 {
		resources.LogLevel("info", "Loading bastion configs....")
		c.Bastion.PublicIPv4Addr = terraform.Output(t, terraformOptions, "bastion_ip")
		c.Bastion.PublicDNS = terraform.Output(t, terraformOptions, "bastion_dns")
	}

	c.ServerIPs = strings.Split(terraform.Output(t, terraformOptions, "master_ips"), ",")
	if c.NumAgents > 0 {
		c.AgentIPs = strings.Split(terraform.Output(t, terraformOptions, "worker_ips"), ",")
	}
}

func loadAws(t *testing.T, varDir string, c *provisioning.Cluster) {
	c.Aws.Region = terraform.GetVariableAsStringFromVarFile(t, varDir, "region")
	c.Aws.AccessKeyID = os.Getenv("AWS_ACCESS_KEY_ID")
	c.Aws.SecretAccessKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	c.Aws.Subnets = terraform.GetVariableAsStringFromVarFile(t, varDir, "subnets")
	c.Aws.AvailabilityZone = terraform.GetVariableAsStringFromVarFile(t, varDir, "availability_zone")
	c.Aws.SgId = terraform.GetVariableAsStringFromVarFile(t, varDir, "sg_id")
	c.Aws.VPCID = terraform.GetVariableAsStringFromVarFile(t, varDir, "vpc_id")
}

func loadEC2(t *testing.T, varDir string, c *provisioning.Cluster) {
	c.Aws.AccessKeyID = terraform.GetVariableAsStringFromVarFile(t, varDir, "access_key")
	c.SSH.User = terraform.GetVariableAsStringFromVarFile(t, varDir, "aws_user")
	c.Aws.EC2.Ami = terraform.GetVariableAsStringFromVarFile(t, varDir, "aws_ami")
	c.Aws.EC2.VolumeSize = terraform.GetVariableAsStringFromVarFile(t, varDir, "volume_size")
	c.Aws.EC2.InstanceClass = terraform.GetVariableAsStringFromVarFile(t, varDir, "ec2_instance_class")
	c.Aws.EC2.KeyName = terraform.GetVariableAsStringFromVarFile(t, varDir, "key_name")
}

func loadExternalDb(t *testing.T, varDir string, c *provisioning.Cluster, terraformOptions *terraform.Options) {
	c.Config.ExternalDbEndpoint = terraform.Output(t, terraformOptions, "rendered_template")
	c.Config.ExternalDb = terraform.GetVariableAsStringFromVarFile(t, varDir, "external_db")
	c.Config.ExternalDbVersion = terraform.GetVariableAsStringFromVarFile(t, varDir, "external_db_version")
	c.Config.ExternalDbGroupName = terraform.GetVariableAsStringFromVarFile(t, varDir, "db_group_name")
	c.Config.ExternalDbNodeType = terraform.GetVariableAsStringFromVarFile(t, varDir, "instance_class")
}
