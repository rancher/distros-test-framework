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
	once      sync.Once
	singleton *Cluster
)

type Cluster struct {
	Status           string
	ServerIPs        []string
	AgentIPs         []string
	WinAgentIPs      []string
	NumServers       int
	NumAgents        int
	NumWinAgents	 int
	RenderedTemplate string
	ExternalDb       string
	ClusterType      string
	ProductType      string
	ArchType         string
}

func loadConfig() (*config.ProductConfig, error) {
	cfg, err := config.LoadConfigEnv("./config")
	if err != nil {
		return nil, fmt.Errorf("error loading env config: %w", err)
	}

	return &cfg, nil
}

func addTerraformOptions(cfg *config.ProductConfig) (*terraform.Options, string, error) {
	var varDir string
	var tfDir string
	var err error

	if cfg.Product == "k3s" || cfg.Product == "rke2" {
		varDir, err = filepath.Abs(shared.BasePath() + fmt.Sprintf("/distros-test-framework/config/%s.tfvars", cfg.Product))
		if err != nil {
			return nil, "", err
		}
		tfDir, err = filepath.Abs(shared.BasePath() + fmt.Sprintf("/distros-test-framework/modules/%s",cfg.Product))
		if err != nil {
			return nil, "", err
		}
	} else {
		return nil, "", fmt.Errorf("invalid product %s", cfg.Product)
	}

	terraformOptions := &terraform.Options{
		TerraformDir: tfDir,
		VarFiles:     []string{varDir},
	}

	return terraformOptions, varDir, nil
}

func addClusterConfig(
	cfg *config.ProductConfig,
	g GinkgoTInterface,
	varDir string,
	terraformOptions *terraform.Options,
) (*Cluster, error) {
	c := &Cluster{}
	var agentIPs []string

	if cfg.Product == "k3s" {
		c.ClusterType = terraform.GetVariableAsStringFromVarFile(g, varDir, "cluster_type")
		c.ExternalDb = terraform.GetVariableAsStringFromVarFile(g, varDir, "external_db")
		c.RenderedTemplate = terraform.Output(g, terraformOptions, "rendered_template")
	} 
	
	shared.KubeConfigFile = terraform.Output(g, terraformOptions, "kubeconfig")
	shared.AwsUser = terraform.GetVariableAsStringFromVarFile(g, varDir, "aws_user")
	shared.AccessKey = terraform.GetVariableAsStringFromVarFile(g, varDir, "access_key")
	shared.Arch = terraform.GetVariableAsStringFromVarFile(g, varDir, "arch")
	c.ArchType = shared.Arch
	c.ProductType = cfg.Product
	
	serverIPs := strings.Split(terraform.Output(g, terraformOptions, "master_ips"), ",")
	c.ServerIPs = serverIPs

	rawAgentIPs := terraform.Output(g, terraformOptions, "worker_ips")
	if rawAgentIPs != "" {
		agentIPs = strings.Split(rawAgentIPs, ",")
		c.AgentIPs = agentIPs
	}
	
	if cfg.Product == "rke2" {
		var winAgentIPs []string
		rawWinAgentIPs := terraform.Output(g, terraformOptions, "windows_worker_ips")
		if rawWinAgentIPs != "" {
			winAgentIPs = strings.Split(rawWinAgentIPs, ",")
			c.WinAgentIPs = winAgentIPs
		}
	}
	
	return c, nil
}

func addSplitRole(g GinkgoTInterface, varDir string, NumServers int) (int, error) {
	splitRoles := terraform.GetVariableAsStringFromVarFile(g, varDir, "split_roles")
	if splitRoles == "true" {
		etcdNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir, "etcd_only_nodes"))
		if err != nil {
			return 0, err
		}
		etcdCpNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir, "etcd_cp_nodes"))
		if err != nil {
			return 0, err
		}
		etcdWorkerNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir, "etcd_worker_nodes"))
		if err != nil {
			return 0, err
		}
		cpNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir, "cp_only_nodes"))
		if err != nil {
			return 0, err
		}
		cpWorkerNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir, "cp_worker_nodes"))
		if err != nil {
			return 0, err
		}
		NumServers = NumServers + etcdNodes + etcdCpNodes + etcdWorkerNodes + cpNodes + cpWorkerNodes
	}

	return NumServers, nil
}
