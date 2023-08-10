package factory

import (
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
	Status     string
	ServerIPs  []string
	AgentIPs   []string
	NumServers int
	NumAgents  int
	Config     ClusterConfig
}

type ClusterConfig struct {
	RenderedTemplate string
	ExternalDb       string
	ClusterType      string
}

func loadConfig() (*config.ProductConfig, error) {
	cfg, err := config.AddConfigEnv("./config")
	if err != nil {
		return nil, shared.ReturnLogError("error getting config: %w", err)
	}

	return cfg, nil
}

func addTerraformOptions() (*terraform.Options, string, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, "", shared.ReturnLogError("error loading config: %w", err)
	}

	var varDir string
	var tfDir string

	if cfg.Product == "rke2" {
		varDir, err = filepath.Abs(shared.BasePath() + "/distros-test-framework/config/rke2.tfvars")
		tfDir, err = filepath.Abs(shared.BasePath() + "/distros-test-framework/modules/rke2")
	} else if cfg.Product == "k3s" {
		varDir, err = filepath.Abs(shared.BasePath() + "/distros-test-framework/config/k3s.tfvars")
		tfDir, err = filepath.Abs(shared.BasePath() + "/distros-test-framework/modules/k3s")
	} else {
		return nil, "", shared.ReturnLogError("invalid product %s", cfg.Product)
	}

	if err != nil {
		return nil, "", shared.ReturnLogError("error getting absolute path: %w", err)
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
	var agentIPs []string

	if cfg.Product == "k3s" {
		c.Config.ClusterType = terraform.GetVariableAsStringFromVarFile(g, varDir, "cluster_type")
		c.Config.ExternalDb = terraform.GetVariableAsStringFromVarFile(g, varDir, "external_db")
		c.Config.RenderedTemplate = terraform.Output(g, terraformOptions, "rendered_template")
		shared.KubeConfigFile = "/tmp/" + terraform.Output(g, terraformOptions, "kubeconfig") + "_kubeconfig"
	} else {
		shared.KubeConfigFile = terraform.Output(g, terraformOptions, "kubeconfig")
	}

	rawAgentIPs := terraform.Output(g, terraformOptions, "worker_ips")
	if rawAgentIPs != "" {
		agentIPs = strings.Split(rawAgentIPs, ",")
	}
	c.AgentIPs = agentIPs

	serverIPs := strings.Split(terraform.Output(g, terraformOptions, "master_ips"), ",")
	c.ServerIPs = serverIPs

	shared.AwsUser = terraform.GetVariableAsStringFromVarFile(g, varDir, "aws_user")
	shared.AccessKey = terraform.GetVariableAsStringFromVarFile(g, varDir, "access_key")

	return c, nil
}

func addSplitRole(g GinkgoTInterface, varDir string, numServers int) (int, error) {
	splitRoles := terraform.GetVariableAsStringFromVarFile(g, varDir, "split_roles")
	if splitRoles == "true" {
		etcdNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir, "etcd_only_nodes"))
		if err != nil {
			return 0, shared.ReturnLogError("error getting etcd_only_nodes %w", err)
		}
		etcdCpNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir, "etcd_cp_nodes"))
		if err != nil {
			return 0, shared.ReturnLogError("error getting etcd_cp_nodes %w", err)
		}
		etcdWorkerNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir, "etcd_worker_nodes"))
		if err != nil {
			return 0, shared.ReturnLogError("error getting etcd_worker_nodes %w", err)
		}
		cpNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir, "cp_only_nodes"))
		if err != nil {
			return 0, shared.ReturnLogError("error getting cp_only_nodes %w", err)
		}
		cpWorkerNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir, "cp_worker_nodes"))
		if err != nil {
			return 0, shared.ReturnLogError("error getting cp_worker_nodes %w", err)
		}
		numServers = numServers + etcdNodes + etcdCpNodes + etcdWorkerNodes + cpNodes + cpWorkerNodes
	}

	return numServers, nil
}
