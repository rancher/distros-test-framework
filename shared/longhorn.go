package shared

import (
	"fmt"
	"time"

	"github.com/avast/retry-go"
)

// InstallLonghorn adds the Longhorn Helm repository and installs the Longhorn storage class and resources to the cluster.
func InstallLonghorn() error {
	if err := InstallLonghornPrerequisites(); err != nil {
		return err
	}

	LogLevel("info", "Installing storage class: longhorn")
	repoCmd := "helm repo add longhorn https://charts.longhorn.io && helm repo update"
	if _, err := RunCommandHost(repoCmd); err != nil {
		return fmt.Errorf("failed to add longhorn helm repo: %w", err)
	}

	installCmd := "helm install longhorn longhorn/longhorn --namespace longhorn --create-namespace --kubeconfig=" + KubeConfigFile
	if _, err := RunCommandHost(installCmd); err != nil {
		return fmt.Errorf("failed to install longhorn: %w", err)
	}

	return VerifyLonghornInstallation()
}

// InstallLonghornPrerequisites installs the required packages (open-iscsi, nfs-common) on all cluster nodes.
func InstallLonghornPrerequisites() error {
	ips := FetchNodeExternalIPs()
	if len(ips) == 0 {
		return fmt.Errorf("no nodes found to install prerequisites")
	}

	for _, ip := range ips {
		LogLevel("info", "Installing pre-requisites: open-iscsi and nfs-common on node %s", ip)
		cmd := `
			if command -v apt-get >/dev/null; then
				sudo apt-get update && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y open-iscsi nfs-common
			elif command -v yum >/dev/null; then
				sudo yum install -y iscsi-initiator-utils nfs-utils
			elif command -v zypper >/dev/null; then
				sudo zypper install -y open-iscsi nfs-client
			fi && sudo modprobe iscsi_tcp
		`
		if _, err := RunCommandOnNode(cmd, ip); err != nil {
			return fmt.Errorf("failed to install prerequisites on node %s: %w", ip, err)
		}
	}
	return nil
}

// VerifyLonghornInstallation checks if Longhorn is successfully deployed and running.
func VerifyLonghornInstallation() error {
	if err := WaitForLonghornReady(); err != nil {
		return fmt.Errorf("longhorn did not become ready: %w", err)
	}

	LogLevel("info", "Verifying longhorn storage class")
	scCmd := "kubectl get sc/longhorn -A -o yaml --kubeconfig=" + KubeConfigFile + " | grep 'storageclass.kubernetes.io/is-default-class'"
	if res, err := RunCommandHost(scCmd); err != nil {
		return fmt.Errorf("failed to verify longhorn storage class: %w, output: %s", err, res)
	}

	LogLevel("info", "Checking all resources in longhorn namespace")
	allCmd := "kubectl get all -n longhorn --kubeconfig=" + KubeConfigFile
	res, err := RunCommandHost(allCmd)
	if err != nil {
		return fmt.Errorf("failed to get longhorn resources: %w, output: %s", err, res)
	}
	LogLevel("debug", "Longhorn resources: \n%s", res)

	LogLevel("info", "Checking sc, pv, pvc across all namespaces")
	scpvCmd := "kubectl get sc,pv,pvc -A --kubeconfig=" + KubeConfigFile
	res, err = RunCommandHost(scpvCmd)
	if err != nil {
		return fmt.Errorf("failed to get storage resources: %w, output: %s", err, res)
	}
	LogLevel("debug", "Storage resources: \n%s", res)

	return nil
}

// WaitForLonghornReady actively monitors the status of Longhorn pods to ensure they come up successfully.
// It times out after 3 minutes.
func WaitForLonghornReady() error {
	LogLevel("info", "Waiting up to 3 minutes for longhorn pods to be in Running state")

	return retry.Do(
		func() error {
			filters := map[string]string{
				"namespace": "longhorn",
			}
			pods, err := GetPodsFiltered(filters)
			if err != nil {
				return fmt.Errorf("failed to get longhorn pods: %w", err)
			}

			if len(pods) == 0 {
				return fmt.Errorf("no longhorn pods found yet")
			}

			for _, pod := range pods {
				if pod.Status != "Running" && pod.Status != "Completed" {
					return fmt.Errorf("pod %s is in state %s", pod.Name, pod.Status)
				}
			}
			return nil
		},
		retry.Attempts(18),
		retry.Delay(10*time.Second),
		retry.DelayType(retry.FixedDelay),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			LogLevel("debug", "Attempt %d: Longhorn not ready yet, retrying... (%v)", n+1, err)
		}),
	)
}
