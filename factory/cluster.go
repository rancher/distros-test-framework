package factory

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/logger"
)

var l = logger.AddLogger()

// ClusterConfig returns a singleton cluster with all terraform config and vars
func ClusterConfig() *Cluster {
	once.Do(func() {
		var err error
		cluster, err = newCluster()
		if err != nil {
			l.Errorf("error creating cluster: %v\n", err)
			return
		}
	})

	return cluster
}

// newCluster creates a new cluster and returns his values from terraform config and vars
func newCluster() (*Cluster, error) {
	cfg, err := config.AddEnv()
	if err != nil {
		return nil, fmt.Errorf("error loading config: %w", err)
	}

	terraformOptions, varDir, err := addTerraformOptions(cfg)
	if err != nil {
		return nil, err
	}

	t := &testing.T{}
	numServers, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"no_of_server_nodes",
	))
	if err != nil {
		return nil, fmt.Errorf(
			"error getting no_of_server_nodes from var file: %w", err)
	}

	numAgents, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"no_of_worker_nodes",
	))
	if err != nil {
		return nil, fmt.Errorf(
			"error getting no_of_worker_nodes from var file: %w\n", err)
	}

	l.Infof("\nCreating cluster\n")
	_, err = terraform.InitAndApplyE(t, terraformOptions)
	if err != nil {
		l.Errorf("\nCreating cluster Failed!!!\n")
		return nil, fmt.Errorf("error creating cluster: %w", err)
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
func DestroyCluster() (string, error) {
	cfg, err := config.AddEnv()
	if err != nil {
		return "", err
	}

	_, callerFilePath, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(callerFilePath), "..")
	varDir, err := filepath.Abs(dir +
		fmt.Sprintf("/config/%s.tfvars", cfg.Product))
	if err != nil {
		return "", fmt.Errorf("invalid product: %s\n", cfg.Product)
	}

	tfDir, err := filepath.Abs(dir +
		fmt.Sprintf("/modules/%s", cfg.Product))
	if err != nil {
		return "", fmt.Errorf("no module found for product: %s\n", cfg.Product)
	}

	terraformOptions := terraform.Options{
		TerraformDir: tfDir,
		VarFiles:     []string{varDir},
	}
	terraform.Destroy(&testing.T{}, &terraformOptions)

	return "cluster destroyed", nil
}
