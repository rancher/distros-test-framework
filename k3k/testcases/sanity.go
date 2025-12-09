package testcases

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
	"github.com/rancher/distros-test-framework/k3k/shared"
)

func TestK3KInstall(host *driver.HostCluster, scType string, useYamlForK3kInstall bool, valuesYamlPath string, k3kNamespace string) error {
	var applyErr, verifyErr error
	setupErr := shared.SetupBinNKubeconfig(host)
	if setupErr != nil {
		return resources.ReturnLogError("setting up competion bash and binary path to PATH vars failed")
	}

	res, err := resources.InstallHelmOnNode(host.ServerIP)
	if err != nil {
		resources.ReturnLogError("error while installing helm: \nResponse:\n%s\nError:\n%v\n", res, err)
	}
	resources.LogLevel("debug", "helm version: %v", res)

	switch strings.ToLower(scType) {
	case "local-path":
		applyErr = shared.ApplyStorageClass("local-path", host)
		if applyErr != nil {
			return resources.ReturnLogError("apply storageclass local-path failed: \nError:\n%w\n", applyErr)
		}
		verifyErr = shared.VerifyStorageClass("local-path", host)
		if verifyErr != nil {
			return resources.ReturnLogError("verify storageclass local-path status failed: \nError:\n%w\n", verifyErr)
		}
	case "longhorn":
		applyErr = shared.ApplyStorageClass("longhorn", host)
		if applyErr != nil {
			return resources.ReturnLogError("apply storageclass longhorn failed: \nError:\n%w\n", applyErr)
		}
		verifyErr = shared.VerifyStorageClass("longhorn", host)
		if verifyErr != nil {
			return resources.ReturnLogError("verify storageclass longhorn status failed: \nError:\n%w\n", verifyErr)
		}
	default:
		fmt.Printf("Unknown storage class type: %s. Skipping storage class installation.\n", scType)
	}

	installErr := shared.InstallK3kcli(host)
	if installErr != nil {
		return resources.ReturnLogError("install k3kcli failed with error: \n%w\n", installErr)
	}

	if k3kNamespace == "" {
		k3kNamespace = os.Getenv("K3K_NAMESPACE")
		if k3kNamespace == "" {
			k3kNamespace = "k3k-system"
		}
		resources.LogLevel("debug", "reset k3knamespace var before installing k3k to: %s", k3kNamespace)
	}
	installErr = shared.InstallK3k(host, useYamlForK3kInstall, valuesYamlPath, k3kNamespace)
	if installErr != nil {
		return resources.ReturnLogError("install k3k failed with error: \n%w\n", installErr)
	}
	return nil
}

func TestK3KClusterCreate(clusterOptions driver.K3kClusterOptions, host *driver.HostCluster) error {
	// Create k3k cluster based on the provided options
	createErr := shared.CreateK3kCluster(clusterOptions, host)
	if createErr != nil {
		resources.ReturnLogError("Create K3K Cluster %s failed with error: \n%w\n", clusterOptions.K3kCluster.Name, createErr)
	}
	// Verify the cluster is created successfully
	verifyErr := shared.VerifyK3KClusterStatus(clusterOptions.K3kCluster, host)
	if verifyErr != nil {
		resources.ReturnLogError("Verify K3K Cluster %s status failed with error: \n%w\n", clusterOptions.K3kCluster.Name, verifyErr)
	}
	resources.PrintGetAllForK3k(host, clusterOptions.K3kCluster.Namespace, host.GetKubectlPath())
	resources.GetResourcesForK3k(true, host.ServerIP, host.KubeconfigPath, clusterOptions.K3kCluster.Namespace, "all")
	resources.GetResourcesForK3k(true, host.ServerIP, host.KubeconfigPath, "", "sc,pv,pvc")
	return nil
}

func TestK3KClusterDelete(k3kcluster driver.K3kCluster, host *driver.HostCluster) error {
	// Delete the k3k cluster based on the provided identifier
	// Verify the cluster is deleted successfully
	deleteErr := shared.DeleteK3kCluster(k3kcluster, host)
	if deleteErr != nil {
		resources.ReturnLogError("Create K3K Cluster %s failed with error: \n%w\n", k3kcluster.Name, deleteErr)
	}
	return nil
}
