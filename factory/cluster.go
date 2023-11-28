package factory

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/gruntwork-io/terratest/modules/terraform"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
)

// ClusterConfig returns a singleton cluster with all terraform config and vars
func ClusterConfig(g GinkgoTInterface) *Cluster {
	once.Do(func() {
		var err error
		cluster, err = newCluster(g)
		if err != nil {
			err = shared.ReturnLogError("error getting cluster: %w\n", err)
			g.Errorf("%s", err)
		}
	})

	return cluster
}

// newCluster creates a new cluster and returns his values from terraform config and vars
func newCluster(g GinkgoTInterface) (*Cluster, error) {
	terraformOptions, varDir, err := addTerraformOptions()
	if err != nil {
		return nil, err
	}

	numServers, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		g,
		varDir,
		"no_of_server_nodes",
	))
	if err != nil {
		return nil, shared.ReturnLogError(
			"error getting no_of_server_nodes from var file: %w", err)
	}

	numAgents, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		g,
		varDir,
		"no_of_worker_nodes",
	))
	if err != nil {
		return nil, shared.ReturnLogError(
			"error getting no_of_worker_nodes from var file: %w\n", err)
	}

	shared.LogLevel("info", "\nCreating cluster\n")
	terraform.InitAndApply(g, terraformOptions)

	numServers, err = addSplitRole(g, varDir, numServers)
	if err != nil {
		return nil, err
	}

	c, err := addClusterConfig(g, varDir, terraformOptions)
	if err != nil {
		return nil, err
	}

	c.NumServers = numServers
	c.NumAgents = numAgents
	c.Status = "cluster created"

	return c, nil
}

// DestroyCluster destroys the cluster and returns it
func DestroyCluster(g GinkgoTInterface) (string, error) {
	var varDir string
	configPath, err := shared.EnvDir("factory")
	if err != nil {
		return "", shared.ReturnLogError("error getting config path: %w", err)
	}

	cfg, err := config.AddConfigEnv(configPath)
	if err != nil {
		return "", shared.ReturnLogError("error getting config: %w", err)
	}

	varDir, err = filepath.Abs(shared.BasePath() +
		fmt.Sprintf("/distros-test-framework/config/%s.tfvars", cfg.Product))
	if err != nil {
		return "", shared.ReturnLogError("invalid product: %s\n", cfg.Product)
	}

	tfDir, err := filepath.Abs(shared.BasePath() +
		fmt.Sprintf("/distros-test-framework/modules/%s", cfg.Product))
	if err != nil {
		return "", shared.ReturnLogError("no module found for product: %s\n", cfg.Product)
	}

	terraformOptions := terraform.Options{
		TerraformDir: tfDir,
		VarFiles:     []string{varDir},
	}
	terraform.Destroy(g, &terraformOptions)

	return "cluster destroyed", nil
}
