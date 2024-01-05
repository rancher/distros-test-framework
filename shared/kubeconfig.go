package shared

import (
	"fmt"
	"os"
	"strings"
)

var KubeConfigFile string

func UpdateKubeConfig(newLeaderIP, resourceName, product string) error {
	if resourceName == "" {
		return ReturnLogError("resourceName not sent\n")
	}

	err := updateKubeConfigLocal(newLeaderIP, resourceName, product)
	if err != nil {
		return ReturnLogError("error creating new kubeconfig file: %w\n", err)
	}

	if err = updateKubeConfigRemote(newLeaderIP, product); err != nil {
		return ReturnLogError("failed to update kubeconfig from cluster: %w\n", err)
	}

	LogLevel("info", "kubeconfig files update\n")

	return nil
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
	// get server ip value from `server:` key
	serverIP := strings.Split(string(kubeconfigContent), "server: ")[1]
	// removing newline
	serverIP = strings.Split(serverIP, "\n")[0]
	// removing the https://
	serverIP = strings.Join(strings.Split(serverIP, "https://")[1:], "")
	// removing the port
	serverIP = strings.Split(serverIP, ":")[0]

	LogLevel("info", "Extracted from local kube config file server ip: %s", serverIP)

	return serverIP, string(kubeconfigContent), nil
}

// updateKubeConfigLocal changes the server ip in the local kubeconfig file.
func updateKubeConfigLocal(newServerIP, resourceName, product string) error {
	if newServerIP == "" {
		return ReturnLogError("ip not sent.\n")
	}
	if product == "" {
		return ReturnLogError("product not sent.\n")
	}
	oldServerIP, kubeconfigContent, err := ExtractServerIP(resourceName)
	if err != nil {
		return ReturnLogError("error extracting server ip: %w\n", err)
	}

	path := fmt.Sprintf("/tmp/%s_kubeconfig", resourceName)
	updatedKubeConfig := strings.ReplaceAll(kubeconfigContent, oldServerIP, newServerIP)
	writeErr := os.WriteFile(path, []byte(updatedKubeConfig), 0644)
	if writeErr != nil {
		return ReturnLogError("failed to write updated kubeconfig file: %w\n", writeErr)
	}

	LogLevel("info", "Updated local kubeconfig with ip: %s", newServerIP)

	return nil
}

// updateKubeConfigRemote changes the server ip in the remote kubeconfig file.
func updateKubeConfigRemote(newServerIP, product string) error {
	if newServerIP == "" {
		return ReturnLogError("ip not sent.\n")
	}
	if product == "" {
		return ReturnLogError("product not sent.\n")
	}

	file := fmt.Sprintf("/etc/rancher/%s/%s.yaml", product, product)
	localHost := "127.0.0.1"
	cmd := fmt.Sprintf("sudo sed -i 's/%s/%s/' %s", localHost, newServerIP, file)

	sedResult, err := RunCommandOnNode(cmd, newServerIP)
	if err != nil {
		return ReturnLogError("error creating kubeconfig %w\n", err)
	}
	if sedResult != "" {
		LogLevel("debug", "sed result: %s", sedResult)
	}

	RestartCluster(product, newServerIP)

	LogLevel("info", "Updated remote kubeconfig with ip: %s", newServerIP)

	return nil
}
