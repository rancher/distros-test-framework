package shared

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

func CreateK3kCluster(k3kOptions driver.K3kClusterOptions, host *driver.HostCluster) error {
	var k3kCmd string
	if k3kOptions.UseValuesYAML {
		// Copy valuesYAMLFile to host
		dirPath := resources.BasePath() + "/k3k/yamls/clusterTypes"
		resources.LogLevel("debug", "Directory path for k3k values YAML files: %s", dirPath)
		valuesYamlPath := fmt.Sprintf("%s/%s", dirPath, k3kOptions.ValuesYAMLFile)
		remoteValuesPath := fmt.Sprintf("/tmp/%s", k3kOptions.ValuesYAMLFile)

		scpErr := resources.CopyFileToRemoteNode(host.ServerIP, host.SSH.User, host.SSH.PrivKeyPath, valuesYamlPath, remoteValuesPath)
		if scpErr != nil {
			return resources.ReturnLogError("failed to copy values YAML file to host: %s", k3kOptions.ValuesYAMLFile)
		}
		resources.LogLevel("info", "values YAML file copied to host: %s\n", remoteValuesPath)

		k3kCmd = fmt.Sprintf("%s apply -f %s && k3kcli kubeconfig generate --namespace %s --name %s --kubeconfig %s",
			host.GetKubectlPath(), remoteValuesPath,
			k3kOptions.K3kCluster.Namespace, k3kOptions.K3kCluster.Name, host.KubeconfigPath)
	} else {
		// Construct k3kcli command with parameters
		k3kCmd = fmt.Sprintf(`export KUBECONFIG=%s && \
    k3kcli cluster create --namespace %s --servers %d --mode %s --server-args=\"%s\" \
	--service-cidr=%s --persistence-type %s --storage-class-name %s --version %s %s`,
			host.KubeconfigPath,
			k3kOptions.K3kCluster.Namespace,
			k3kOptions.NoOfServers,
			k3kOptions.Mode,
			k3kOptions.ServerArgs,
			k3kOptions.ServiceCIDR,
			k3kOptions.PersistenceType,
			k3kOptions.StorageClassType,
			k3kOptions.K3SVersion,
			k3kOptions.K3kCluster.Name)
	}

	if k3kOptions.NoOfAgents > 0 {
		resources.LogLevel("debug", "Installing with mode: %s and Appending no of agents: %d", k3kOptions.Mode, k3kOptions.NoOfAgents)
		k3kCmd = k3kCmd + fmt.Sprintf(" --agents %d", k3kOptions.NoOfAgents)
	}

	out, err := resources.RunCommandOnNode(k3kCmd, host.ServerIP)
	if err != nil {
		return resources.ReturnLogError("failed to create k3k cluster: \n%w\n", err)
	}
	resources.LogLevel("info", "k3k cluster created: %s\nOutput:\n%s\n", k3kOptions.K3kCluster.Name, out)

	return nil
}

func DeleteK3kCluster(k3kCluster driver.K3kCluster, host *driver.HostCluster) error {
	k3kCmd := fmt.Sprintf("export KUBECONFIG=%s && k3kcli cluster delete %s -n %s", host.KubeconfigPath, k3kCluster.Name, k3kCluster.Namespace)
	out, err := resources.RunCommandOnNode(k3kCmd, host.ServerIP)
	if err != nil {
		return resources.ReturnLogError("failed to delete k3k cluster: %s; Output: %s; error: %w\n", k3kCmd, out, err)
	}
	resources.LogLevel("info", "k3k cluster deleted: %s\n", k3kCluster.Name)

	if isK3kClusterExists(k3kCluster, host) {
		return resources.ReturnLogError("k3k cluster is still being listed by k3kcli as Ready")
	} else {
		resources.LogLevel("info", "k3kcli does not list the cluster anymore - cluster deleted successfully!")
	}

	return nil
}

func GenerateK3kKubeconfig(k3kcluster driver.K3kCluster, host *driver.HostCluster) (string, error) {
	k3kCmd := fmt.Sprintf("k3kcli kubeconfig generate --namespace %s --name %s", k3kcluster.Namespace, k3kcluster.Name)
	out, err := resources.RunCommandOnNode(k3kCmd, host.ServerIP)
	if err != nil {
		return "", resources.ReturnLogError("failed to generate kubeconfig for k3k cluster: %s; Output: %s; Error: %w\n", k3kcluster.Name, out, err)
	}
	resources.LogLevel("info", "k3k kubeconfig retrieved\n")
	k3kcluster.SetKubeconfigPath(host)
	kubeconfigFilePath := k3kcluster.GetKubeconfigPath(host)

	return kubeconfigFilePath, nil
}

func isK3kClusterExists(cluster driver.K3kCluster, host *driver.HostCluster) bool {
	cmd := fmt.Sprintf("export KUBECONFIG=%s && k3kcli cluster list | grep %s", host.KubeconfigPath, cluster.Name)
	out, err := resources.RunCommandOnNode(cmd, host.ServerIP)
	if err != nil {
		return false
	}

	return strings.Contains(out, cluster.Name) && strings.Contains(out, "Ready")
}

func VerifyK3KClusterStatus(k3kCluster driver.K3kCluster, host *driver.HostCluster) error {
	if isK3kClusterExists(k3kCluster, host) {
		resources.LogLevel("info", "k3k cluster status verified successfully.\n")
	} else {
		return resources.ReturnLogError("k3kcluster %s is either not listed or nor in Ready state", k3kCluster.Name)
	}

	return nil
}
