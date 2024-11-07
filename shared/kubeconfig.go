package shared

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

var KubeConfigFile string

// KubeConfigCluster gets the kubeconfig file decoded.
//
// updates the global kubeconfig and returns the nodes and his data from that kubeconfig.
func KubeConfigCluster(kubeconfig string) *Cluster {
	localKubeConfigPath, decodeErr := decodeKubeConfig(kubeconfig)
	if decodeErr != nil {
		LogLevel("error", "error decoding kubeconfig %v\n", decodeErr)
		os.Exit(1)
	}

	// Set the global kubeconfig file path since on this flow we dont have it created yet.
	KubeConfigFile = localKubeConfigPath

	nodes, getErr := GetNodes(false)
	if getErr != nil {
		LogLevel("error", "error getting nodes: %v\n", getErr)
		os.Exit(1)
	}
	if len(nodes) == 0 {
		LogLevel("error", "no nodes found\n")
		return nil
	}

	var clusterErr error
	cluster, clusterErr = addClusterFromKubeConfig(nodes)
	if clusterErr != nil {
		LogLevel("error", "error adding cluster from kubeconfig %v\n", clusterErr)
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
		return "", ReturnLogError("resourceName not sent\n")
	}

	kubeConfigUpdated, err := updateKubeConfigLocal(newLeaderIP, resourceName, product)
	if err != nil {
		return "", ReturnLogError("error creating new kubeconfig file: %v\n", err)
	}

	return kubeConfigUpdated, nil
}

// ExtractServerIP extracts the server IP from the kubeconfig file.
//
// returns the server ip and the kubeconfigContent as plain string.
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
		return "", ReturnLogError("error extracting server ip: %v\n", err)
	}

	path := fmt.Sprintf("/tmp/%s_kubeconfig", resourceName)
	updatedKubeConfig := strings.ReplaceAll(kubeconfigContent, oldServerIP, newServerIP)

	writeErr := os.WriteFile(path, []byte(updatedKubeConfig), 0o644)
	if writeErr != nil {
		return "", ReturnLogError("failed to write updated kubeconfig file: %v\n", writeErr)
	}

	updatedKubeConfig = base64.StdEncoding.EncodeToString([]byte(updatedKubeConfig))

	return updatedKubeConfig, nil
}

// decodeKubeConfig decodes the kubeconfig and writes it to a local /tmp file.
func decodeKubeConfig(kubeConfig string) (string, error) {
	dec, err := base64.StdEncoding.DecodeString(kubeConfig)
	if err != nil {
		return "", ReturnLogError("failed to decode kubeconfig: %v\n", err)
	}

	localKubeConfigPath := fmt.Sprintf("/tmp/%s_kubeconfig", os.Getenv("resource_name"))
	writeErr := os.WriteFile(localKubeConfigPath, dec, 0o644)
	if writeErr != nil {
		return "", ReturnLogError("failed to write kubeconfig file: %v\n", writeErr)
	}

	return localKubeConfigPath, nil
}
