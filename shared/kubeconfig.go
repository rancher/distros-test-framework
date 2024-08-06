package shared

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

var KubeConfigFile string

func KubeConfigCluster(kubeconfig string) *Cluster {
	nodes, clusterErr := getNodesFromKubeConfig(kubeconfig)
	if clusterErr != nil {
		LogLevel("error", "error getting nodes from kubeconfig %w\n", clusterErr)
		os.Exit(1)
	}

	cluster, clusterErr = addClusterFromKubeConfig(nodes)
	if clusterErr != nil {
		LogLevel("error", "error adding cluster from kubeconfig %w\n", clusterErr)
		os.Exit(1)
	}

	return cluster
}

func UpdateKubeConfig(newLeaderIP, resourceName, product string) (string, error) {
	if resourceName == "" {
		return "", ReturnLogError("resourceName not sent\n")
	}

	kubeConfigUpdated, err := updateKubeConfigLocal(newLeaderIP, resourceName, product)
	if err != nil {
		return "", ReturnLogError("error creating new kubeconfig file: %w\n", err)
	}

	return kubeConfigUpdated, nil
}

// ExtractServerIP extracts the server IP from the kubeconfig file.
func ExtractServerIP(resourceName string) (kubeConfigIP, kubeCfg string, err error) {
	if resourceName == "" {
		return "", "", ReturnLogError("resource name not sent\n")
	}

	localPath := fmt.Sprintf("/tmp/%s_kubeconfig", resourceName)
	kubeconfigContent, err := os.ReadFile(localPath)
	if err != nil {
		return "", "", ReturnLogError("failed to read kubeconfig file: %w\n", err)
	}
	// get server ip value from `server:` key.
	serverIP := strings.Split(string(kubeconfigContent), "server: ")[1]
	// removing newline.
	serverIP = strings.Split(serverIP, "\n")[0]
	// removing the https://.
	serverIP = strings.Join(strings.Split(serverIP, "https://")[1:], "")
	// removing the port.
	serverIP = strings.Split(serverIP, ":")[0]

	return serverIP, string(kubeconfigContent), nil
}

// updateKubeConfigLocal changes the server ip in the local kubeconfig file and returns the updated kubeconfig in base64.
func updateKubeConfigLocal(newServerIP, resourceName, product string) (string, error) {
	if newServerIP == "" {
		return "", ReturnLogError("ip not sent.\n")
	}
	if product == "" {
		return "", ReturnLogError("product not sent.\n")
	}

	oldServerIP, kubeconfigContent, err := ExtractServerIP(resourceName)
	if err != nil {
		return "", ReturnLogError("error extracting server ip: %w\n", err)
	}

	path := fmt.Sprintf("/tmp/%s_kubeconfig", resourceName)
	updatedKubeConfig := strings.ReplaceAll(kubeconfigContent, oldServerIP, newServerIP)

	writeErr := os.WriteFile(path, []byte(updatedKubeConfig), 0o644)
	if writeErr != nil {
		return "", ReturnLogError("failed to write updated kubeconfig file: %w\n", writeErr)
	}

	updatedKubeConfig = base64.StdEncoding.EncodeToString([]byte(updatedKubeConfig))

	return updatedKubeConfig, nil
}

func getNodesFromKubeConfig(kubeConfig string) ([]Node, error) {
	decodeErr := decodeKubeConfig(kubeConfig)
	if decodeErr != nil {
		LogLevel("error", "error decoding kubeconfig: %w\n", decodeErr)
		return nil, decodeErr
	}

	nodes, getErr := GetNodes(false)
	if getErr != nil {
		LogLevel("error", "error getting nodes: %w\n", getErr)
		return nil, getErr
	}
	if len(nodes) == 0 {
		return nil, ReturnLogError("no nodes found\n")
	}

	return nodes, nil
}

func decodeKubeConfig(kubeConfig string) error {
	dec, err := base64.StdEncoding.DecodeString(kubeConfig)
	if err != nil {
		LogLevel("error", "error decoding kubeconfig: %w\n", err)
		return err
	}

	localPath := fmt.Sprintf("/tmp/%s_kubeconfig", os.Getenv("resource_name"))
	writeErr := os.WriteFile(localPath, dec, 0o644)
	if writeErr != nil {
		LogLevel("error", "failed to write kubeconfig file: %w\n", writeErr)
		return writeErr
	}

	KubeConfigFile = localPath

	return nil
}
