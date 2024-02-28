package factory

import (
	"fmt"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/shared"
)

// ClusterConfig returns a singleton cluster with all terraform config and vars
func ClusterConfig(t *testing.T) *Cluster {
	once.Do(func() {
		var err error
		cluster, err = newCluster(t)
		if err != nil {
			err = shared.ReturnLogError("error getting cluster: %w\n", err)
			t.Errorf("%s", err)
		}
	})

	return cluster
}

// newCluster creates a new cluster and returns his values from terraform config and vars
func newCluster(t *testing.T) (*Cluster, error) {
	cfg, err := config.AddEnv()
	if err != nil {
		return nil, shared.ReturnLogError("error loading config: %w\n", err)
	}

	terraformOptions, varDir, err := addTerraformOptions(cfg)
	if err != nil {
		return nil, err
	}

	numServers, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"no_of_server_nodes",
	))
	if err != nil {
		return nil, shared.ReturnLogError(
			"error getting no_of_server_nodes from var file: %w", err)
	}

	numAgents, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"no_of_worker_nodes",
	))
	if err != nil {
		return nil, shared.ReturnLogError(
			"error getting no_of_worker_nodes from var file: %w\n", err)
	}

	shared.LogLevel("info", "\nCreating cluster\n")
	_, err = terraform.InitAndApplyE(t, terraformOptions)
	if err != nil {
		shared.LogLevel("error", "\nCreating cluster Failed!!!\n")
		return nil, err
	}

	numServers, err = addSplitRole(t, varDir, numServers)
	if err != nil {
		return nil, err
	}

	c, err := loadTFconfig(t, varDir, terraformOptions, cfg)
	if err != nil {
		return nil, err
	}

	c.NumServers = numServers
	c.NumAgents = numAgents
	c.Status = "cluster created"

	return c, nil
}

// DestroyCluster destroys the cluster and returns it
func DestroyCluster(t *testing.T) (string, error) {
	var varDir string
	cfg, err := config.AddEnv()
	if err != nil {
		return "", err
	}

	varDir, err = filepath.Abs(shared.BasePath() +
		fmt.Sprintf("/config/%s.tfvars", cfg.Product))
	if err != nil {
		return "", shared.ReturnLogError("invalid product: %s\n", cfg.Product)
	}

	tfDir, err := filepath.Abs(shared.BasePath() +
		fmt.Sprintf("/modules/%s", cfg.Product))
	if err != nil {
		return "", shared.ReturnLogError("no module found for product: %s\n", cfg.Product)
	}

	terraformOptions := terraform.Options{
		TerraformDir: tfDir,
		VarFiles:     []string{varDir},
	}
	terraform.Destroy(t, &terraformOptions)

	return "cluster destroyed", nil
}
