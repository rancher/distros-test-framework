package shared

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
)

func addTerraformOptions(product string) (*terraform.Options, string, error) {
	_, callerFilePath, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(callerFilePath), "..")

	varDir, err := filepath.Abs(dir +
		fmt.Sprintf("/config/%s.tfvars", product))
	if err != nil {
		return nil, "", fmt.Errorf("invalid product: %s\n", product)
	}

	tfDir, err := filepath.Abs(dir + "/modules/" + product)
	if err != nil {
		return nil, "", fmt.Errorf("no module found for product: %s\n", product)
	}

	terraformOptions := &terraform.Options{
		TerraformDir: tfDir,
		VarFiles:     []string{varDir},
	}

	return terraformOptions, varDir, nil
}

func loadTFconfig(
	t *testing.T,
	varDir string,
	terraformOptions *terraform.Options,
	product string,
) (*Cluster, error) {
	c := &Cluster{}

	loadTFoutput(t, terraformOptions, c)
	loadAwsEC2(t, varDir, c)
	if product == "rke2" {
		loadWinTFCfg(t, varDir, terraformOptions, c)
	}

	c.Config.Arch = terraform.GetVariableAsStringFromVarFile(t, varDir, "arch")
	c.Config.Product = product

	c.Config.DataStore = terraform.GetVariableAsStringFromVarFile(t, varDir, "datastore_type")
	if c.Config.DataStore == "external" {
		c.Config.ExternalDb = terraform.GetVariableAsStringFromVarFile(t, varDir, "external_db")
		c.Config.RenderedTemplate = terraform.Output(t, terraformOptions, "rendered_template")
	}

	return c, nil
}

func loadAwsEC2(t *testing.T, varDir string, c *Cluster) {
	c.AwsEC2.AccessKey = terraform.GetVariableAsStringFromVarFile(t, varDir, "access_key")
	c.AwsEC2.AwsUser = terraform.GetVariableAsStringFromVarFile(t, varDir, "aws_user")
	c.AwsEC2.Ami = terraform.GetVariableAsStringFromVarFile(t, varDir, "aws_ami")
	c.AwsEC2.Region = terraform.GetVariableAsStringFromVarFile(t, varDir, "region")
	c.AwsEC2.VolumeSize = terraform.GetVariableAsStringFromVarFile(t, varDir, "volume_size")
	c.AwsEC2.InstanceClass = terraform.GetVariableAsStringFromVarFile(t, varDir, "ec2_instance_class")
	c.AwsEC2.Subnets = terraform.GetVariableAsStringFromVarFile(t, varDir, "subnets")
	c.AwsEC2.AvailabilityZone = terraform.GetVariableAsStringFromVarFile(t, varDir, "availability_zone")
	c.AwsEC2.SgId = terraform.GetVariableAsStringFromVarFile(t, varDir, "sg_id")
	c.AwsEC2.KeyName = terraform.GetVariableAsStringFromVarFile(t, varDir, "key_name")
}

func loadTFoutput(t *testing.T, terraformOptions *terraform.Options, c *Cluster) {
	KubeConfigFile = terraform.Output(t, terraformOptions, "kubeconfig")
	c.GeneralConfig.BastionIP = terraform.Output(t, terraformOptions, "bastion_ip")
	c.FQDN = terraform.Output(t, terraformOptions, "Route53_info")
	c.ServerIPs = strings.Split(terraform.Output(t, terraformOptions, "master_ips"), ",")
	rawAgentIPs := terraform.Output(t, terraformOptions, "worker_ips")
	if rawAgentIPs != "" {
		c.AgentIPs = strings.Split(rawAgentIPs, ",")
	}
}

func loadWinTFCfg(t *testing.T, varDir string, terraformOptions *terraform.Options, c *Cluster) {
	rawWinAgentIPs := terraform.Output(t, terraformOptions, "windows_worker_ips")
	if rawWinAgentIPs != "" {
		c.WinAgentIPs = strings.Split(rawWinAgentIPs, ",")
	}

	numWinAgents, _ := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(t, varDir, "no_of_windows_worker_nodes"))
	c.NumWinAgents = numWinAgents
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
