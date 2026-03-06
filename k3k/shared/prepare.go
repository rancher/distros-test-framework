package shared

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

var LonghornNamespace = "longhorn"
var LocalpathNamespace = "local-path-storage"

func InstallK3kcli(host *driver.HostCluster) error {
	resources.LogLevel("debug", "Checking if k3kcli is installed first. If error occurs, proceeding to install k3kcli.")
	if !isK3kcliInstalled(host) {
		k3kcliCmd := fmt.Sprintf(`wget -qO k3kcli https://github.com/rancher/k3k/releases/download/%s/k3kcli-linux-amd64 && \
	chmod +x k3kcli && \
	sudo mv k3kcli /usr/local/bin/k3kcli && \
	k3kcli -v`, host.K3kcliVersion)
		out, err := resources.RunCommandOnNode(k3kcliCmd, host.ServerIP)
		if err != nil {
			return resources.ReturnLogError(fmt.Sprintf("failed to install k3kcli! \n Command: \n %s\nError: %s\n", k3kcliCmd, err))
		}
		resources.LogLevel("info", "k3kcli installed! \nOutput:\n%s\n", out)
	} else {
		resources.LogLevel("info", "k3kcli is already installed!")
	}
	return nil
}

func isK3kcliInstalled(host *driver.HostCluster) bool {
	cmd := "k3kcli -v"
	out, err := resources.RunCommandOnNode(cmd, host.ServerIP)
	if err != nil {
		resources.LogLevel("error", "Cmd: %s; \nOutput:\n%s\nError:\n%w\n", cmd, out, err)
		return false
	}

	isTrue := strings.Contains(out, os.Getenv("K3KCLI_VERSION"))
	resources.LogLevel("debug", "k3kcli version: %s \nreturn isK3kcliInstalled: %t\n", out, isTrue)
	return isTrue
}

func isK3kInstalled(k3kNamespace string, host *driver.HostCluster) bool {
	cmd := fmt.Sprintf("%s get ns/%s", host.GetKubectlPath(), k3kNamespace)
	out, err := resources.RunCommandOnNode(cmd, host.ServerIP)
	if err != nil {
		resources.LogLevel("debug", "Get namespace %s returned with error - return isK3KInstalled: false", k3kNamespace)
		return false
	}

	isTrue := strings.Contains(out, k3kNamespace)
	resources.LogLevel("debug", "Get namespace %s output check: return isK3KInstalled: %t", k3kNamespace, isTrue)
	return isTrue
}

func isStorageClassInstalled(scType string, host *driver.HostCluster) bool {
	var namespace string
	if scType == "local-path" {
		namespace = LocalpathNamespace
	} else {
		namespace = LonghornNamespace

	}
	nscmd := fmt.Sprintf("%s get ns/%s", host.GetKubectlPath(), namespace)
	outns, errns := resources.RunCommandOnNode(nscmd, host.ServerIP)
	if errns != nil {
		resources.LogLevel("debug", "Get namespace %s returned with error - return isStorageClassInstalled: false", namespace)
		return false
	}

	sccmd := fmt.Sprintf("%s get sc/%s", host.GetKubectlPath(), scType)
	scout, scerr := resources.RunCommandOnNode(sccmd, host.ServerIP)
	if scerr != nil {
		resources.LogLevel("debug", "Get storageclass %s returned with error - return isStorageClassInstalled: false", scType)
		return false
	}
	resources.LogLevel("debug", "namespace %s found? %t", namespace, strings.Contains(outns, namespace))
	resources.LogLevel("debug", "sc %s found? %t", scType, strings.Contains(scout, scType))
	return strings.Contains(outns, namespace) && strings.Contains(scout, scType)
}

func InstallK3k(host *driver.HostCluster, useValuesYaml bool, valuesYamlPath string, k3kNamespace string) error {
	resources.LogLevel("debug", "Checking if k3k is installed first. If error occurs, proceeding to install k3k.")

	if !isK3kInstalled(k3kNamespace, host) {
		setINotifyCmd := `sudo sysctl -w fs.inotify.max_user_instances=2099999999 && \
  sudo sysctl -w fs.inotify.max_user_watches=2099999999 && \
  sudo sysctl -w fs.inotify.max_queued_events=2099999999`
		_, err := resources.RunCommandOnNode(setINotifyCmd, host.ServerIP)
		if err != nil {
			return resources.ReturnLogError("failed to set inotify limits: \n %s; \nError: %w\n", setINotifyCmd, err)
		}
		resources.LogLevel("info", "inotify limits set!  \n")

		var valuesCmdAppend string
		if useValuesYaml {
			remotePath := fmt.Sprintf("/home/%s/values.yaml", host.SSH.User)
			scpErr := resources.CopyFileToRemoteNode(host.ServerIP, host.SSH.User, host.SSH.PubKeyPath, valuesYamlPath, remotePath)
			if scpErr != nil {
				return resources.ReturnLogError("failed to copy file: %w", scpErr)
			}
			resources.LogLevel("info", "values.yaml copied to remote!  \n")
			valuesCmdAppend = fmt.Sprintf("-f %s", remotePath)
		} else {
			valuesCmdAppend = ""
		}

		k3kCmd := fmt.Sprintf(`helm repo add k3k https://rancher.github.io/k3k && \
		helm repo update && \
		export KUBECONFIG=%s && \
		helm install --namespace %s --create-namespace k3k k3k/k3k --devel %s`,
			host.KubeconfigPath,
			k3kNamespace,
			valuesCmdAppend)
		_, err = resources.RunCommandOnNode(k3kCmd, host.ServerIP)
		if err != nil {
			return resources.ReturnLogError("failed to install k3k Cmd: %s; \nError: \n%s", k3kCmd, err)
		}
		resources.LogLevel("info", "k3k installed successfully!  \n")
	} else {
		resources.LogLevel("info", "k3k is already installed with namespace %s - re-using the same!  \n", k3kNamespace)
	}

	return nil
}

func installOpenScsi(host *driver.HostCluster) error {
	// TODO: add for RHEL/SLES OS based command options as well
	openScsiCmd := "sudo apt-get install -y open-iscsi && sudo modprobe iscsi_tcp"
	_, err := resources.RunCommandOnNode(openScsiCmd, host.ServerIP)
	if err != nil {
		return resources.ReturnLogError("failed to install open-iscsi: %s; \nError: %s", openScsiCmd, err)
	}
	resources.LogLevel("info", "open-iscsi installed!  \n")

	return nil
}

func installNFSCommon(host *driver.HostCluster) error {
	// TODO: add for RHEL/SLES OS based command options as well
	nfsCommonCmd := "sudo apt-get install -y nfs-common"
	_, err := resources.RunCommandOnNode(nfsCommonCmd, host.ServerIP)
	if err != nil {
		return errors.Wrapf(err, "failed to install nfs-common")
	}
	resources.LogLevel("info", "nfs-common installed!  \n")

	return nil
}

func ApplyStorageClass(scType string, host *driver.HostCluster) error {
	var scYAML string
	var cmd string

	resources.LogLevel("debug", "Checking if storageclass is installed first. If error occurs, proceeding to install.")

	if isStorageClassInstalled(scType, host) {
		resources.LogLevel("info", "storage class %s already exists. re-using the previous installation!", scType)
		return nil
	}

	if strings.ToLower(scType) == "local-path" {
		scYAML = "https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml"
		cmd = fmt.Sprintf("%s apply -f %s", host.GetKubectlPath(), scYAML)
	} else if strings.ToLower(scType) == "longhorn" {
		installOpenScsi(host)
		installNFSCommon(host)
		cmd = fmt.Sprintf(`helm repo add longhorn https://charts.longhorn.io && \
  helm repo update && \
  export KUBECONFIG=%s && \
  helm install longhorn longhorn/longhorn --namespace %s --create-namespace`, host.KubeconfigPath, LonghornNamespace)
	} else {
		return resources.ReturnLogError("unsupported storage class type: %s", scType)
	}
	resources.LogLevel("debug", "Running Install storage class %s with command: \n%s\n", scType, cmd)

	out, err := resources.RunCommandOnNode(cmd, host.ServerIP)
	if err != nil {
		return resources.ReturnLogError("failed to apply %s storage class on node %s\n Error: %s", scType, host.ServerIP, err)
	}
	resources.LogLevel("info", "%s storage class applied on node %s\n Output: %s", scType, host.ServerIP, out)

	err = WaitForStorageClassPodsToRun(host, scType)
	if err != nil {
		return resources.ReturnLogError("error waiting for storage class %s pods to run: %w", scType, err)
	}

	return nil
}

func getStorageClassNamespace(scType string) string {
	if strings.ToLower(scType) == "local-path" {
		return LocalpathNamespace
	} else if strings.ToLower(scType) == "longhorn" {
		return LonghornNamespace
	} else {
		return ""
	}
}

func logStorageClassPodsStatus(host *driver.HostCluster, scType string) {
	var cmd string
	scNamespace := getStorageClassNamespace(scType)

	cmd = fmt.Sprintf("kubectl get all -n %s --kubeconfig=%s", scNamespace, host.KubeconfigPath)
	out, err := resources.RunCommandOnNode(cmd, host.ServerIP)
	if err != nil {
		resources.LogLevel("error", "Get all resources for storage class %s Output: \n%s\n Error: \n%w\n", scType, out, err)
	}
	resources.LogLevel("info", "Storage class all resource list output: \n%s\n", out)
}

func WaitForStorageClassPodsToRun(host *driver.HostCluster, scType string) error {
	var waitPodsErr error
	scNamespace := getStorageClassNamespace(scType)
	waitPodsErr = resources.MonitorPodsStatus(host.ServerIP, host.KubeconfigPath, scNamespace, 30*time.Second, 10)
	if waitPodsErr != nil {
		resources.ReturnLogError("pods for %s namespace not up after 5 minutes", scType)
	}
	resources.LogLevel("info", "Pods are up for storage class: %s", scType)

	return nil
}

func VerifyStorageClass(scType string, host *driver.HostCluster) error {
	if isStorageClassInstalled(scType, host) {
		resources.LogLevel("info", "%s storage class exists on node.", scType)
	} else {
		err := WaitForStorageClassPodsToRun(host, scType)
		if err != nil {
			return resources.ReturnLogError("error waiting for storage class %s pods to run: %w", scType, err)
		}
	}

	logStorageClassPodsStatus(host, scType)

	return nil
}

func patchStorageClassToDefault(host *driver.HostCluster, scType string, isDefault bool) error {
	// Patch the storage class to be the default storage class
	resources.LogLevel("debug", "Update storageclass %s to default as %t", scType, isDefault)
	patchValue := fmt.Sprintf(`{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"%t"}}}`, isDefault)
	patchCmd := fmt.Sprintf("%s patch storageclass %s -p '%s'", host.GetKubectlPath(), scType, patchValue)
	out, err := resources.RunCommandOnNode(patchCmd, host.ServerIP)
	if err != nil {
		return resources.ReturnLogError("failed to patch storageclass %s as %t; \n Error: \n%s\n", scType, isDefault, err)
	} else {
		resources.LogLevel("info", "default storageclass %s patched to be  %t\nOutput: \n %s\n", scType, isDefault, string(out))
	}

	return nil
}

func PatchDefaultStorageClass(host *driver.HostCluster, scType string) error {
	if scType == "local-path" {
		patchStorageClassToDefault(host, "longhorn", false)
		patchStorageClassToDefault(host, "local-path", true)
	} else { // longhorn
		patchStorageClassToDefault(host, "local-path", false)
		patchStorageClassToDefault(host, "longhorn", true)
	}

	return nil
}

func SetupBinNKubeconfig(host *driver.HostCluster) error {
	var cmd string
	if host.HostClusterType == "rke2" {
		cmd = fmt.Sprintf("%s completion bash --kubectl --crictl --ctr -i", host.HostClusterType)
	} else {
		cmd = fmt.Sprintf("%s completion bash -i", host.HostClusterType)
	}
	out, err := resources.RunCommandOnNode(cmd, host.ServerIP)
	if err != nil {
		return resources.ReturnLogError("failed to run completion bash: \nOutput:\n%s\n%w\n", out, err)
	}
	resources.LogLevel("info", "run completion bash successfully!")
	if host.HostClusterType == "rke2" {
		cmd := fmt.Sprintf("grep -qF 'export PATH=$PATH:/var/lib/rancher' ~/.bashrc || echo '\nexport PATH=$PATH:/var/lib/rancher/%s/bin:/opt/%s/bin:/mnt/bin\n' >> ~/.bashrc", host.HostClusterType, host.HostClusterType)
		out, err := resources.RunCommandOnNode(cmd, host.ServerIP)
		if err != nil {
			return resources.ReturnLogError(`failed to update PATH var to include /var/lib/rancher/%s/bin and /opt/%s/bin \nOutput:\n%s\nError:\n%w\n`,
				host.HostClusterType, host.HostClusterType, out, err)
		}
		resources.LogLevel("info", "updated PATH var to include binary paths /var/lib/rancher/%s/bin and /opt/%s/bin successfully!", host.HostClusterType, host.HostClusterType)
	}

	return nil
}
