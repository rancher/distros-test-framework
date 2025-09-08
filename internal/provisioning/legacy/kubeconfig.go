package legacy

import (
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// KubeConfigCluster gets the kubeconfig file decoded.
//
// updates the global kubeconfig and returns the nodes and his data from that kubeconfig.
func KubeConfigCluster(kubeconfig string) *driver.Cluster {
	localKubeConfigPath, decodeErr := decodeKubeConfig(kubeconfig)
	if decodeErr != nil {
		resources.LogLevel("error", "error decoding kubeconfig %v\n", decodeErr)
		os.Exit(1)
	}

	// Set the global kubeconfig file path as it's not created for this flow.
	resources.KubeConfigFile = localKubeConfigPath

	nodes, getErr := resources.GetNodes(false)
	if getErr != nil {
		resources.LogLevel("error", "error getting nodes: %v\n", getErr)
		os.Exit(1)
	}
	if len(nodes) == 0 {
		resources.LogLevel("error", "no nodes found\n")
		return nil
	}

	var clusterErr error
	cluster, clusterErr = addClusterFromKubeConfig(nodes)

	if clusterErr != nil {
		resources.LogLevel("error", "error adding cluster from kubeconfig %v\n", clusterErr)
		os.Exit(1)
	}

	return cluster
}

// UpdateKubeConfig updates the kubeconfig file with the new leader ip.
//
// the path used by KubeConfigFile is maintained.
//
// returns the updated kubeconfig in base64.
func UpdateKubeConfig(newLeaderIP, resourceName, product string) (string, error) {
	if resourceName == "" {
		return "", resources.ReturnLogError("resourceName not sent\n")
	}

	kubeConfigUpdated, err := updateKubeConfigLocal(newLeaderIP, resourceName, product)
	if err != nil {
		return "", resources.ReturnLogError("error creating new kubeconfig file: %v\n", err)
	}

	return kubeConfigUpdated, nil
}

// updateKubeConfigLocal changes the server ip in the local kubeconfig file and returns the updated kubeconfig in base64.
func updateKubeConfigLocal(newServerIP, resourceName, product string) (string, error) {
	if newServerIP == "" {
		return "", resources.ReturnLogError("ip not sent.\n")
	}
	if product == "" {
		return "", resources.ReturnLogError("product not sent.\n")
	}

	oldServerIP, kubeconfigContent, err := resources.ExtractServerIP(resourceName)
	if err != nil {
		return "", resources.ReturnLogError("error extracting server ip: %v\n", err)
	}

	path := fmt.Sprintf("/tmp/%s_kubeconfig", resourceName)
	updatedKubeConfig := strings.ReplaceAll(kubeconfigContent, oldServerIP, newServerIP)

	writeErr := os.WriteFile(path, []byte(updatedKubeConfig), 0o644)
	if writeErr != nil {
		return "", resources.ReturnLogError("failed to write updated kubeconfig file: %v\n", writeErr)
	}

	updatedKubeConfig = base64.StdEncoding.EncodeToString([]byte(updatedKubeConfig))

	return updatedKubeConfig, nil
}

// NewLocalKubeconfigFile get the remote cluster kubeconfig file updates the server ip and writes it to a local file.
//
// also sets the global KubeConfigFile pointing to the new kubeconfig file.
func NewLocalKubeconfigFile(newServerIP, resourceName, product, localPath string) error {
	if newServerIP == "" {
		return resources.ReturnLogError("ip not sent.\n")
	}
	if product == "" {
		return resources.ReturnLogError("product not sent.\n")
	}
	if resourceName == "" {
		return resources.ReturnLogError("resourceName not sent.\n")
	}
	if localPath == "" {
		return resources.ReturnLogError("path not sent.\n")
	}

	cmd := fmt.Sprintf("sudo cat /etc/rancher/%s/%s.yaml", product, product)
	kubeconfigContent, err := resources.RunCommandOnNode(cmd, newServerIP)
	if err != nil {
		return resources.ReturnLogError("failed to get kubeconfig file: %v\n", err)
	}

	serverIPRgx := regexp.MustCompile(`server: https://\d+\.\d+\.\d+\.\d+`)
	replace := "server: https://" + newServerIP
	updated := serverIPRgx.ReplaceAllString(kubeconfigContent, replace)

	writeErr := os.WriteFile(localPath, []byte(updated), 0o644)
	if writeErr != nil {
		return resources.ReturnLogError("failed to write updated kubeconfig file: %v\n", writeErr)
	}

	resources.KubeConfigFile = localPath
	resources.LogLevel("info", "kubeconfig var updated: %s\n", resources.KubeConfigFile)

	return nil
}

// decodeKubeConfig decodes the kubeconfig and writes it to a local /tmp file.
func decodeKubeConfig(kubeConfig string) (string, error) {
	dec, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(kubeConfig, " ", ""))
	if err != nil {
		return "", resources.ReturnLogError("failed to decode kubeconfig: %v\n", err)
	}

	localKubeConfigPath := fmt.Sprintf("/tmp/%s_kubeconfig", os.Getenv("resource_name"))
	writeErr := os.WriteFile(localKubeConfigPath, dec, 0o644)
	if writeErr != nil {
		return "", resources.ReturnLogError("failed to write kubeconfig file: %v\n", writeErr)
	}

	return localKubeConfigPath, nil
}
