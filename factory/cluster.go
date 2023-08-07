package factory

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
)

// NewCluster creates a new cluster and returns his values from terraform config and vars
func NewCluster(g GinkgoTInterface) (*Cluster, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}

	terraformOptions, varDir, err := addTerraformOptions(cfg)
	if err != nil {
		return nil, err
	}

	NumServers, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir, "no_of_server_nodes"))
	if err != nil {
		return nil, err
	}

	NumAgents, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir, "no_of_worker_nodes"))
	if err != nil {
		return nil, err
	}

	NumWinAgents, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir, "no_of_windows_worker_nodes"))
	if err != nil {
		return nil, err
	}

	fmt.Println("Creating Cluster")
	terraform.InitAndApply(g, terraformOptions)

	NumServers, err = addSplitRole(g, varDir, NumServers)
	if err != nil {
		return nil, err
	}

	c, err := addClusterConfig(cfg, g, varDir, terraformOptions)
	if err != nil {
		return nil, err
	}

	c.NumServers = NumServers
	c.NumAgents = NumAgents
	c.NumWinAgents = NumWinAgents
	c.Status = "cluster created"

	return c, nil
}

// GetCluster returns a singleton cluster
func GetCluster(g GinkgoTInterface) *Cluster {
	var err error
	once.Do(func() {
		singleton, err = NewCluster(g)
		if err != nil {
			g.Errorf("error getting cluster: %v", err)
		}
	})
	return singleton
}

// DestroyCluster destroys the cluster and returns a message
func DestroyCluster(g GinkgoTInterface) (string, error) {
	var varDir string

	cfg, err := loadConfig()
	if err != nil {
		return "", fmt.Errorf("error loading config: %w", err)
	}

	tfDir, err := filepath.Abs(shared.BasePath() + "/distros-test-framework/modules")
	if err != nil {
		return "", err
	}

	if cfg.Product == "rke2" || cfg.Product == "k3s"{
		varDir, err = filepath.Abs(shared.BasePath() + fmt.Sprintf("/distros-test-framework/config/%s.tfvars", cfg.Product))
		if err != nil {
			return "", err
		}
	} else {
		return "", fmt.Errorf("invalid product %s", cfg.Product)
	}

	terraformOptions := terraform.Options{
		TerraformDir: tfDir,
		VarFiles:     []string{varDir},
	}
	terraform.Destroy(g, &terraformOptions)

	return "cluster destroyed", nil
}
