package factory

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gruntwork-io/terratest/modules/terraform"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
)

var (
	once    sync.Once
	cluster *Cluster
)

type Cluster struct {
	Status       string
	ServerIPs    []string
	AgentIPs     []string
	WinAgentIPs  []string
	NumWinAgents int
	NumServers   int
	NumAgents    int
	Config       clusterConfig
	AwsEc2       awsEc2Config
}

type awsEc2Config struct {
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

func loadConfig() (*config.Product, error) {
	cfg, err := shared.EnvConfig()
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func addTerraformOptions() (*terraform.Options, string, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, "", shared.ReturnLogError("error loading config: %w\n", err)
	}

	var varDir string
	var tfDir string

	varDir, err = filepath.Abs(shared.BasePath() +
		fmt.Sprintf("/config/%s.tfvars", cfg.Product))
	if err != nil {
		return nil, "", shared.ReturnLogError("invalid product: %s\n", cfg.Product)
	}

	tfDir, err = filepath.Abs(shared.BasePath() +
		fmt.Sprintf("/modules/%s", cfg.Product))
	if err != nil {
		return nil, "", shared.ReturnLogError("no module found for product: %s\n", cfg.Product)
	}

	terraformOptions := &terraform.Options{
		TerraformDir: tfDir,
		VarFiles:     []string{varDir},
	}

	return terraformOptions, varDir, nil
}

func addClusterConfig(
	g GinkgoTInterface,
	varDir string,
	terraformOptions *terraform.Options,
) (*Cluster, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, shared.ReturnLogError("error loading config: %w", err)
	}

	c := &Cluster{}

	shared.KubeConfigFile = terraform.Output(g, terraformOptions, "kubeconfig")
	shared.AwsUser = terraform.GetVariableAsStringFromVarFile(g, varDir, "aws_user")
	shared.AccessKey = terraform.GetVariableAsStringFromVarFile(g, varDir, "access_key")
	shared.Arch = terraform.GetVariableAsStringFromVarFile(g, varDir, "arch")

	c.AwsEc2.Ami = terraform.GetVariableAsStringFromVarFile(g, varDir, "aws_ami")
	c.AwsEc2.Region = terraform.GetVariableAsStringFromVarFile(g, varDir, "region")
	c.AwsEc2.VolumeSize = terraform.GetVariableAsStringFromVarFile(g, varDir, "volume_size")
	c.AwsEc2.InstanceClass = terraform.GetVariableAsStringFromVarFile(g, varDir, "ec2_instance_class")
	c.AwsEc2.Subnets = terraform.GetVariableAsStringFromVarFile(g, varDir, "subnets")
	c.AwsEc2.AvailabilityZone = terraform.GetVariableAsStringFromVarFile(g, varDir, "availability_zone")
	c.AwsEc2.SgId = terraform.GetVariableAsStringFromVarFile(g, varDir, "sg_id")
	c.AwsEc2.KeyName = terraform.GetVariableAsStringFromVarFile(g, varDir, "key_name")

	c.Config.Arch = shared.Arch
	c.Config.Product = cfg.Product

	c.ServerIPs = strings.Split(terraform.Output(g, terraformOptions, "master_ips"), ",")

	if cfg.Product == "k3s" {
		c.Config.DataStore = terraform.GetVariableAsStringFromVarFile(g, varDir, "datastore_type")
		if c.Config.DataStore == "external" {
			c.Config.ExternalDb = terraform.GetVariableAsStringFromVarFile(g, varDir, "external_db")
			c.Config.RenderedTemplate = terraform.Output(g, terraformOptions, "rendered_template")
		} else if c.Config.DataStore == "" {
			return nil, shared.ReturnLogError("datastore should not be empty \n%w", err)
		}
	}

	rawAgentIPs := terraform.Output(g, terraformOptions, "worker_ips")
	if rawAgentIPs != "" {
		c.AgentIPs = strings.Split(rawAgentIPs, ",")
	}

	if cfg.Product == "rke2" {
		rawWinAgentIPs := terraform.Output(g, terraformOptions, "windows_worker_ips")
		if rawWinAgentIPs != "" {
			c.WinAgentIPs = strings.Split(rawWinAgentIPs, ",")
		}
		numWinAgents, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir,
			"no_of_windows_worker_nodes"))
		if err != nil {
			return nil, shared.ReturnLogError("error getting no_of_windows_worker_nodes: \n%w", err)
		}

		c.NumWinAgents = numWinAgents
	}

	return c, nil
}

func addSplitRole(g GinkgoTInterface, varDir string, numServers int) (int, error) {
	splitRoles := terraform.GetVariableAsStringFromVarFile(g, varDir, "split_roles")
	if splitRoles == "true" {
		etcdNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
			g,
			varDir,
			"etcd_only_nodes",
		))
		if err != nil {
			return 0, shared.ReturnLogError("error getting etcd_only_nodes %w", err)
		}
		etcdCpNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
			g,
			varDir,
			"etcd_cp_nodes",
		))
		if err != nil {
			return 0, shared.ReturnLogError("error getting etcd_cp_nodes %w", err)
		}
		etcdWorkerNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
			g,
			varDir,
			"etcd_worker_nodes",
		))
		if err != nil {
			return 0, shared.ReturnLogError("error getting etcd_worker_nodes %w", err)
		}
		cpNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
			g,
			varDir,
			"cp_only_nodes",
		))
		if err != nil {
			return 0, shared.ReturnLogError("error getting cp_only_nodes %w", err)
		}
		cpWorkerNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
			g,
			varDir,
			"cp_worker_nodes",
		))
		if err != nil {
			return 0, shared.ReturnLogError("error getting cp_worker_nodes %w", err)
		}
		numServers = numServers + etcdNodes + etcdCpNodes + etcdWorkerNodes + cpNodes + cpWorkerNodes
	}

	return numServers, nil
}
