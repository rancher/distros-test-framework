package factory

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"

	"github.com/rancher/distros-test-framework/config"
)

var (
	once           sync.Once
	cluster        *Cluster
	KubeConfigFile string
)

type Cluster struct {
	Status        string
	ServerIPs     []string
	AgentIPs      []string
	WinAgentIPs   []string
	NumWinAgents  int
	NumServers    int
	NumAgents     int
	FQDN          string
	Config        clusterConfig
	AwsConfig     awsConfig
	GeneralConfig generalConfig
}

type awsConfig struct {
	AccessKey        string
	AwsUser          string
	Ami              string
	Region           string
	VolumeSize       string
	InstanceClass    string
	Subnets          string
	AvailabilityZone string
	SgId             string
	KeyName          string
}

type clusterConfig struct {
	RenderedTemplate string
	ExternalDb       string
	DataStore        string
	Product          string
	Arch             string
}

type generalConfig struct {
	BastionIP string
}

func addTerraformOptions(cfg *config.Product) (*terraform.Options, string, error) {
	_, callerFilePath, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(callerFilePath), "..")

	varDir, err := filepath.Abs(dir +
		fmt.Sprintf("/config/%s.tfvars", cfg.Product))
	if err != nil {
		return nil, "", fmt.Errorf("invalid product: %s\n", cfg.Product)
	}

	tfDir, err := filepath.Abs(dir +
		fmt.Sprintf("/modules/%s", cfg.Product))
	if err != nil {
		return nil, "", fmt.Errorf("no module found for product: %s\n", cfg.Product)
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
	cfg *config.Product,
) (*Cluster, error) {
	c := &Cluster{}

	loadTFoutput(t, terraformOptions, c)
	loadAwsConfig(t, varDir, c)

	c.Config.Arch = terraform.GetVariableAsStringFromVarFile(t, varDir, "arch")
	c.Config.Product = cfg.Product

	var err error
	if c.Config.Product == "k3s" {
		err = loadK3sCfg(t, varDir, terraformOptions, c)
	} else {
		err = loadRke2Cfg(t, varDir, terraformOptions, c)
	}
	if err != nil {
		l.Errorf("error loading %s config\n", c.Config.Product)
		return nil, err
	}

	return c, nil
}

func loadRke2Cfg(t *testing.T, varDir string, terraformOptions *terraform.Options, c *Cluster) error {
	rawWinAgentIPs := terraform.Output(t, terraformOptions, "windows_worker_ips")
	if rawWinAgentIPs != "" {
		c.WinAgentIPs = strings.Split(rawWinAgentIPs, ",")
	}
	numWinAgents, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(t, varDir, "no_of_windows_worker_nodes"))
	if err != nil {
		return fmt.Errorf("error getting no_of_windows_worker_nodes: \n%w", err)
	}
	c.NumWinAgents = numWinAgents

	return nil
}

func loadK3sCfg(t *testing.T, varDir string, terraformOptions *terraform.Options, c *Cluster) error {
	c.Config.DataStore = terraform.GetVariableAsStringFromVarFile(t, varDir, "datastore_type")
	if c.Config.DataStore == "external" {
		c.Config.ExternalDb = terraform.GetVariableAsStringFromVarFile(t, varDir, "external_db")
		c.Config.RenderedTemplate = terraform.Output(t, terraformOptions, "rendered_template")
	}

	return nil
}

func loadAwsConfig(t *testing.T, varDir string, c *Cluster) {
	c.AwsConfig.AccessKey = terraform.GetVariableAsStringFromVarFile(t, varDir, "access_key")
	c.AwsConfig.AwsUser = terraform.GetVariableAsStringFromVarFile(t, varDir, "aws_user")
	c.AwsConfig.Ami = terraform.GetVariableAsStringFromVarFile(t, varDir, "aws_ami")
	c.AwsConfig.Region = terraform.GetVariableAsStringFromVarFile(t, varDir, "region")
	c.AwsConfig.VolumeSize = terraform.GetVariableAsStringFromVarFile(t, varDir, "volume_size")
	c.AwsConfig.InstanceClass = terraform.GetVariableAsStringFromVarFile(t, varDir, "ec2_instance_class")
	c.AwsConfig.Subnets = terraform.GetVariableAsStringFromVarFile(t, varDir, "subnets")
	c.AwsConfig.AvailabilityZone = terraform.GetVariableAsStringFromVarFile(t, varDir, "availability_zone")
	c.AwsConfig.SgId = terraform.GetVariableAsStringFromVarFile(t, varDir, "sg_id")
	c.AwsConfig.KeyName = terraform.GetVariableAsStringFromVarFile(t, varDir, "key_name")
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
