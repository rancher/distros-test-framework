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

	if product == "rke2" {
		LogLevel("info", "Loading Windows tf outputs...")
		loadWinTFOutput(t, terraformOptions, cluster, varDir)
	}

	LogLevel("info", "Loading other tfvars in to config....")
	c.Config.Arch = terraform.GetVariableAsStringFromVarFile(t, varDir, "arch")
	c.Config.Product = product
	c.Config.ServerFlags = terraform.GetVariableAsStringFromVarFile(t, varDir, "server_flags")
	c.Config.DataStore = terraform.GetVariableAsStringFromVarFile(t, varDir, "datastore_type")
	if c.Config.DataStore == "external" {
		c.Config.ExternalDb = terraform.GetVariableAsStringFromVarFile(t, varDir, "external_db")
		c.Config.RenderedTemplate = terraform.Output(t, terraformOptions, "rendered_template")
	}
	if module == "airgap" || module == "ipv6only" {
		c.Config.Version = terraform.GetVariableAsStringFromVarFile(t, varDir, "install_version")
	}

	return c, nil
}

func loadAws(t *testing.T, varDir string, c *Cluster) {
	c.Aws.Region = terraform.GetVariableAsStringFromVarFile(t, varDir, "region")
	c.Aws.AccessKeyID = os.Getenv("AWS_ACCESS_KEY_ID")
	c.Aws.SecretAccessKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
}

func loadEC2(t *testing.T, varDir string, c *Cluster) {
	c.Aws.EC2.AccessKey = terraform.GetVariableAsStringFromVarFile(t, varDir, "access_key")
	c.Aws.EC2.AwsUser = terraform.GetVariableAsStringFromVarFile(t, varDir, "aws_user")
	c.Aws.EC2.Ami = terraform.GetVariableAsStringFromVarFile(t, varDir, "aws_ami")
	c.Aws.EC2.VolumeSize = terraform.GetVariableAsStringFromVarFile(t, varDir, "volume_size")
	c.Aws.EC2.InstanceClass = terraform.GetVariableAsStringFromVarFile(t, varDir, "ec2_instance_class")
	c.Aws.EC2.Subnets = terraform.GetVariableAsStringFromVarFile(t, varDir, "subnets")
	c.Aws.EC2.AvailabilityZone = terraform.GetVariableAsStringFromVarFile(t, varDir, "availability_zone")
	c.Aws.EC2.SgId = terraform.GetVariableAsStringFromVarFile(t, varDir, "sg_id")
	c.Aws.EC2.KeyName = terraform.GetVariableAsStringFromVarFile(t, varDir, "key_name")
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

func loadWinTFOutput(t *testing.T, tfOpts *terraform.Options, c *Cluster, varDir string) {
	numWinAgents, _ := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t, varDir, "no_of_windows_worker_nodes"))
	c.NumWinAgents = numWinAgents
	if c.NumWinAgents > 0 {
		LogLevel("info", "Loading windows TF Config...")
		c.WinAgentIPs = strings.Split(terraform.Output(t, tfOpts, "windows_worker_ips"), ",")
	}
}

func addSplitRole(t *testing.T, varDir string, numServers int) (int, error) {
	splitRoles := terraform.GetVariableAsStringFromVarFile(t, varDir, "split_roles")
	if splitRoles == "true" {
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
	}

	return numServers, nil
}
