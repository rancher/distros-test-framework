package testcase

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"golang.org/x/crypto/ssh"

	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestNvidia(cluster *shared.Cluster) {
	targetNodeIP := cluster.ServerIPs[0]
	checkOS := "cat /etc/os-release"

	osName, cmdErr := shared.RunCommandOnNode(checkOS, targetNodeIP)
	Expect(cmdErr).NotTo(HaveOccurred(), "error checking OS version: %v", cmdErr)

	if strings.Contains(osName, "rhel") {
		shared.LogLevel("info", "Proceeding with RHEL setup for NVIDIA driver installation")
		initialSetupRHEL(targetNodeIP)
	} else if strings.Contains(osName, "sles") {
		shared.LogLevel("info", "Proceeding with SLES setup for NVIDIA driver installation")
		initialSetupSles(targetNodeIP)
	}
	shared.LogLevel("info", "Successfully installing nvidia driver and compute utils")

	workloadErr := shared.ManageWorkload("apply", "nvidia-operator.yaml")
	Expect(workloadErr).NotTo(HaveOccurred(), "nvidia operator manifests not deployed")
	workloadErr = shared.ManageWorkload("apply", "nvidia-benchmark.yaml")
	Expect(workloadErr).NotTo(HaveOccurred(), "nvidia benchmark manifests not deployed")

	validateNvidiaModule(targetNodeIP)
	validateNvidiaVersion(targetNodeIP)
	validateNvidiaLibMl(targetNodeIP)

	nodeName, err := shared.RunCommandHost("kubectl get nodes -o jsonpath='{.items[0].metadata.name}' " +
		"--kubeconfig=" + shared.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(), "failed to get node name: %v", err)
	Expect(nodeName).NotTo(BeEmpty(), "Node name is empty")

	validateNvidiaOperatorDeploy(nodeName)

	validateNvidiaGPU(nodeName)

	validateNvidiaRunBinPath(targetNodeIP)

	validateContainerdConfig(targetNodeIP)

	validateNvidiaToolKit(targetNodeIP)

	validateNvidiaPodStatus()

	validateBenchmark()
}

func initialSetupSles(ip string) {
	installDriver := "sudo zypper -v --non-interactive in 'nvidia-open-driver-G06-signed-cuda-kmp==570.124.06'"
	_, installDriverErr := shared.RunCommandOnNode(installDriver, ip)
	Expect(installDriverErr).ToNot(HaveOccurred(), "error installing driver: %v", installDriverErr)

	cudaRepo := "sudo zypper ar https://developer.download.nvidia.com/compute/cuda/repos/sles15/x86_64 cuda"
	_, cudaRepoErr := shared.RunCommandOnNode(cudaRepo, ip)
	if cudaRepoErr != nil && !strings.Contains(cudaRepoErr.Error(), "exists") {
		Expect(cudaRepoErr).NotTo(HaveOccurred(), "error adding cuda repo: %v", cudaRepoErr)
	} else if cudaRepoErr != nil {
		shared.LogLevel("warn", "CUDA repo already exists, proceeding...")
	}
	shared.LogLevel("info", "Added CUDA repository")

	gpgKeys := "sudo zypper --gpg-auto-import-keys ref"
	_, gpgKeysErr := shared.RunCommandOnNode(gpgKeys, ip)
	Expect(gpgKeysErr).ToNot(HaveOccurred(), "error importing gpg keys: %v", gpgKeysErr)

	cmdref := "sudo zypper --non-interactive ref"
	res, cmdErr := shared.RunCommandOnNode(cmdref, ip)
	Expect(cmdErr).ToNot(HaveOccurred(), "error refreshing repos: %v", cmdErr)
	Expect(res).To(ContainSubstring("All repositories have been refreshed."))

	keyImport := "sudo rpm --import " +
		"https://developer.download.nvidia.com/compute/cuda/repos/sles15/x86_64/repodata/repomd.xml.key"
	_, keyImportErr := shared.RunCommandOnNode(keyImport, ip)
	Expect(keyImportErr).ToNot(HaveOccurred(), "error importing key: %v", keyImportErr)
	shared.LogLevel("info", "Imported NVIDIA RPM key")

	installComputeUtils := "sudo zypper -v --non-interactive in -r cuda 'nvidia-compute-utils-G06==570.124.06'"
	_, installComputeUtilsErr := shared.RunCommandOnNode(installComputeUtils, ip)
	Expect(installComputeUtilsErr).ToNot(HaveOccurred(), "error installing compute utils: %v", installComputeUtilsErr)
}

func initialSetupRHEL(ip string) {
	installPackages := "sudo dnf -y install " +
		"kernel-devel-$(uname -r) " +
		"kernel-headers-$(uname -r) " +
		"gcc make elfutils-libelf-devel libglvnd-devel"

	_, cmdErr := shared.RunCommandOnNode(installPackages, ip)
	Expect(cmdErr).ToNot(HaveOccurred(), "error installing pre-requisite packages: %v", cmdErr)
	shared.LogLevel("info", "Installed pre-requisite packages")

	cmdRepo := "sudo dnf config-manager --add-repo" +
		" https://developer.download.nvidia.com/compute/cuda/repos/rhel8/x86_64/cuda-rhel8.repo"
	_, cmdErr = shared.RunCommandOnNode(cmdRepo, ip)
	Expect(cmdErr).ToNot(HaveOccurred(), "error adding CUDA repo: %v", cmdErr)

	installDriver := " sudo dnf module install nvidia-driver:565-dkms"
	_, cmdErr = shared.RunCommandOnNode(installDriver, ip)
	Expect(cmdErr).ToNot(HaveOccurred(), "error enabling nvidia-driver module: %v", cmdErr)

	clean := "sudo dnf clean all && sudo dnf makecache "
	_, cmdErr = shared.RunCommandOnNode(clean, ip)
	Expect(cmdErr).ToNot(HaveOccurred(), "error cleaning dnf cache: %v", cmdErr)

	keyImport := "sudo rpm --import https://developer.download.nvidia.com/compute/cuda/repos/rhel8/x86_64/D42D0685.pub"
	_, keyImportErr := shared.RunCommandOnNode(keyImport, ip)
	Expect(keyImportErr).ToNot(HaveOccurred(), "error importing NVIDIA GPG key: %v", keyImportErr)
	shared.LogLevel("info", "Imported NVIDIA GPG key")

	repoRefresh := "sudo dnf clean all && sudo dnf makecache"
	_, repoRefreshErr := shared.RunCommandOnNode(repoRefresh, ip)
	Expect(repoRefreshErr).ToNot(HaveOccurred(), "error refreshing repo metadata: %v", repoRefreshErr)

	installComputeUtils := "sudo dnf install -y nvidia-compute-utils-570.124.06"
	_, installComputeUtilsErr := shared.RunCommandOnNode(installComputeUtils, ip)
	Expect(installComputeUtilsErr).ToNot(HaveOccurred(), "error installing compute utils: %v", installComputeUtilsErr)
	shared.LogLevel("info", "Installed NVIDIA compute utilities")

	updateEnvPath := "sudo echo 'export PATH=/usr/local/cuda/bin:$PATH' | sudo tee /etc/profile.d/cuda.sh && " +
		"echo 'export LD_LIBRARY_PATH=/usr/local/cuda/lib64:$LD_LIBRARY_PATH' | " +
		"sudo tee -a /etc/profile.d/cuda.sh && sudo chmod +x /etc/profile.d/cuda.sh"
	_, updateEnvErr := shared.RunCommandOnNode(updateEnvPath, ip)
	Expect(updateEnvErr).ToNot(HaveOccurred(), "error setting environment variables: %v", updateEnvErr)

	createSymlink := "if [ -f /usr/local/cuda/lib64/libnvidia-ml.so ] && [ ! -f /usr/lib64/libnvidia-ml.so ]; " +
		"then sudo ln -sf /usr/local/cuda/lib64/libnvidia-ml.so* /usr/lib64/; fi"
	_, cmdErr = shared.RunCommandOnNode(createSymlink, ip)
	Expect(cmdErr).ToNot(HaveOccurred(), "error creating symlinks: %v", cmdErr)
}

func validateNvidiaModule(ip string) {
	var res string
	var lsmodErr error

	retryErr := retry.Do(
		func() error {
			lsmodCmd := "sudo lsmod | grep nvidia"
			res, lsmodErr = shared.RunCommandOnNode(lsmodCmd, ip)

			if lsmodErr != nil {
				var exitErr *ssh.ExitError
				if errors.As(lsmodErr, &exitErr) && exitErr.ExitStatus() == 1 {
					return errors.New("nvidia module not yet found via lsmod | grep")
				}

				return fmt.Errorf("failed to run lsmod command: %w", lsmodErr)
			}

			if !strings.Contains(res, "nvidia") {
				return errors.New("nvidia module not found in lsmod output")
			}

			return nil
		},
		retry.Attempts(20),
		retry.Delay(10*time.Second),
		retry.OnRetry(func(n uint, err error) {
			shared.LogLevel("warn", "Attempt %d failed, retrying to find nvidia module: %v", n+1, err)
		}),
	)
	Expect(retryErr).NotTo(HaveOccurred(),
		"failed to find nvidia module via lsmod after multiple attempts: %v", retryErr)

	Expect(res).To(ContainSubstring("nvidia"), "lsmod output does not contain 'nvidia'")
	Expect(res).To(Or(ContainSubstring("nvidia_uvm"), ContainSubstring("nvidia_drm")),
		"lsmod output does not contain 'nvidia_uvm' or 'nvidia_drm'")
	Expect(res).To(ContainSubstring("nvidia_modeset"), "lsmod output does not contain 'nvidia_modeset'")

	shared.LogLevel("info", "NVIDIA modules found:\n%s", res)
}

func validateNvidiaVersion(ip string) {
	versionCmd := "sudo cat /proc/driver/nvidia/version"

	res, err := shared.RunCommandOnNode(versionCmd, ip)
	Expect(err).NotTo(HaveOccurred(), "failed to read driver version: %v", err)
	Expect(res).To(ContainSubstring("NVRM version: NVIDIA"), "NVRM version string not found")
	Expect(res).To(ContainSubstring("GCC version:"), "GCC version string not found")
	Expect(res).To(ContainSubstring("Release Build"), "Release Build string not found")
	Expect(res).To(ContainSubstring("NVIDIA UNIX Open Kernel Module"),
		"NVIDIA UNIX Open Kernel Module string not found")

	shared.LogLevel("info", "NVIDIA driver version:\n%s", res)
}

func validateNvidiaLibMl(ip string) {
	findCmd := "sudo find /usr/ -iname libnvidia-ml.so"

	res, err := shared.RunCommandOnNode(findCmd, ip)
	Expect(err).NotTo(HaveOccurred(), "failed to find libnvidia-ml.so: %v", err)
	Expect(res).To(Or(ContainSubstring("/usr/lib64/libnvidia-ml.so"),
		ContainSubstring("/usr/lib/x86_64-linux-gnu/libnvidia-ml.so")), "libnvidia-ml.so not found")

	shared.LogLevel("info", "libnvidia-ml.so found at:\n%s", res)
}

func validateNvidiaOperatorDeploy(nodeName string) {
	delayTime := time.After(120 * time.Second)
	// as per doc , delay time needed.
	// https://docs.rke2.io/advanced#deploy-nvidia-operator
	<-delayTime

	cmd := fmt.Sprintf("kubectl get node %s  --kubeconfig=%s -o jsonpath=\"{.metadata.labels}\" ",
		strings.TrimSpace(nodeName), shared.KubeConfigFile)
	res, labelErr := shared.RunCommandHost(cmd)
	Expect(labelErr).NotTo(HaveOccurred(), "failed to get node labels: %v", labelErr)

	Expect(strings.TrimSpace(res)).To(Not(BeEmpty()), "Label nvidia.com/gpu.deploy.driver not found")
	Expect(strings.TrimSpace(res)).To(ContainSubstring("\"nvidia.com/gpu.deploy.driver\":"+"\"pre-installed\""),
		"wrong label value for nvidia.com/gpu.deploy.driver")

	Expect(res).To(Or(ContainSubstring("nvidia.com/cuda.driver.major"), ContainSubstring("nvidia.com/gpu.machine")),
		"Label nvidia.com/cuda.driver.major OR nvidia.com/gpu.machine not found")
	Expect(res).To(ContainSubstring("nvidia.com/gpu.count"), "Label nvidia.com/gpu.count not found")
	Expect(res).To(ContainSubstring("nvidia.com/gpu.product"), "Label nvidia.com/gpu.product not found")

	shared.LogLevel("debug", "Nvidia operator labels found on node %s:\n%s", nodeName, res)
}

func validateNvidiaGPU(nodeName string) {
	cmd := fmt.Sprintf("kubectl get node %s -o jsonpath=\"{.status.allocatable}\"", nodeName)
	res, err := shared.RunCommandHost(cmd + " --kubeconfig=" + shared.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred())

	gpuRegex := regexp.MustCompile(`"nvidia\.com/gpu":"(\d+)"`)
	ok := gpuRegex.FindStringSubmatch(res)
	Expect(ok).To(HaveLen(2), "Failed to extract GPU count")

	value := ok[1]
	count, err := strconv.Atoi(value)
	Expect(err).NotTo(HaveOccurred(), "failed to convert GPU value to integer")
	Expect(count).To(BeNumerically(">", 0), "GPU count is not greater than 0")

	shared.LogLevel("info", "Nvidia GPU count found on node %s: %d", nodeName, count)
}

func validateNvidiaToolKit(ip string) {
	toolkit := "sudo ls -l /usr/local/nvidia/toolkit | cat"
	res, err := shared.RunCommandOnNode(toolkit, ip)
	Expect(err).NotTo(HaveOccurred(), "failed to list toolkit directory: %v", err)
	Expect(res).To(ContainSubstring("nvidia-container-runtime"),
		"nvidia-container-runtime not found in toolkit directory")

	shared.LogLevel("info", "Nvidia toolkit directory:\n%s", res)
}

func validateContainerdConfig(ip string) {
	containerdConfigPath := "/var/lib/rancher/rke2/agent/etc/containerd/config.toml"
	checkCmd := "sudo grep nvidia  " + containerdConfigPath

	res, err := shared.RunCommandOnNode(checkCmd, ip)
	Expect(err).NotTo(HaveOccurred(), "failed to grep containerd config or 'nvidia' runtime not found in %s",
		containerdConfigPath)
	Expect(res).To(ContainSubstring("nvidia"), "containerd config does not contain nvidia runtime entry")

	shared.LogLevel("info", "Containerd config contains nvidia runtime entry:\n%s", res)
}

func validateNvidiaRunBinPath(ip string) {
	runtimeBinPath := "/usr/local/nvidia/toolkit/nvidia-container-runtime"
	checkCmd := "sudo stat " + runtimeBinPath

	res, err := shared.RunCommandOnNode(checkCmd, ip)
	Expect(err).NotTo(HaveOccurred(), "nvidia-container-runtime binary not found at %s", runtimeBinPath)

	shared.LogLevel("info", "nvidia-container-runtime binary found at %s", res)
}

func validateNvidiaPodStatus() {
	cmd := fmt.Sprintf("kubectl get pod nbody-gpu-benchmark -n kube-system --kubeconfig=%s -o jsonpath='{.status.phase}'",
		shared.KubeConfigFile)
	podStatus, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed to get benchmark pod status: %v", err)

	podStatus = strings.TrimSpace(podStatus)
	Expect(podStatus).To(Or(Equal("Succeeded"), Equal("Running"), Equal("Completed")),
		"Benchmark pod phase is not Succeeded/Running/Completed")

	shared.LogLevel("info", "Benchmark pod status:\n%s", podStatus)
}

func validateBenchmark() {
	benchmarkLogs := "kubectl logs nbody-gpu-benchmark -n kube-system --kubeconfig=" + shared.KubeConfigFile
	logs, logErr := shared.RunCommandHost(benchmarkLogs)
	Expect(logErr).NotTo(HaveOccurred(), "Failed to get benchmark pod logs")

	Expect(logs).To(ContainSubstring(" CUDA device: [Tesla T4]"),
		"CUDA device not found in benchmark logs")
	Expect(logs).To(ContainSubstring("billion interactions per second"),
		"Benchmark logs did not contain 'billion interactions per second info'")

	shared.LogLevel("info", "Benchmark logs contain nvidia device and performance info:\n%s", logs)
}
