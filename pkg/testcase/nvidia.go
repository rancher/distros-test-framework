package testcase

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go"

	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

var nodeOs string

func TestNvidiaIntegration(cluster *shared.Cluster) {
	targetNodeIP := cluster.ServerIPs[0]
	nodeOs = cluster.NodeOS

	switch nodeOs {
	case "ubuntu":
		shared.LogLevel("info", "Proceeding with Ubuntu setup for NVIDIA driver installation")
		initialSetupUbuntu(targetNodeIP)
	case "rhel", "rhel8", "rhel9":
		shared.LogLevel("info", "Proceeding with RHEL setup for NVIDIA driver installation")
		initialSetupRHEL(targetNodeIP)
	case "sles15":
		shared.LogLevel("info", "Proceeding with SLES setup for NVIDIA driver installation")
		initialSetupSles(targetNodeIP)
	default:
		shared.LogLevel("error", "Unsupported OS: %s", nodeOs)
		return
	}

	workloadErr := shared.ManageWorkload("apply", "nvidia-operator.yaml")
	Expect(workloadErr).NotTo(HaveOccurred(), "nvidia operator manifests not deployed")

	restartRke2 := "sudo systemctl restart rke2-server.service"
	_, restartRke2Err := shared.RunCommandOnNode(restartRke2, targetNodeIP)
	if restartRke2Err != nil {
		shared.LogLevel("warn", "Error restarting rke2: %v", restartRke2Err)
	}

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

	err = validateNvidiaModule(targetNodeIP)
	Expect(err).NotTo(HaveOccurred(), "NVIDIA module not found: %v", err)

	workloadErr = shared.ManageWorkload("apply", "nvidia-benchmark.yaml")
	Expect(workloadErr).NotTo(HaveOccurred(), "nvidia benchmark manifests not deployed")
	validateNvidiaBenchmarkPodStatus()
	validateBenchmark()
}

func initialSetupUbuntu(ip string) {
	updateCmd := "sudo apt update"
	_, updateErr := shared.RunCommandOnNode(updateCmd, ip)
	Expect(updateErr).ToNot(HaveOccurred(), "error updating package lists: %v", updateErr)
	shared.LogLevel("info", "Updated package lists")

	installPrereqs := "DEBIAN_FRONTEND=noninteractive sudo apt install -y build-essential " +
		"linux-headers-$(uname -r) " +
		"pkg-config " +
		"libglvnd-dev " +
		"xorg-dev " +
		"vulkan-tools " +
		"dkms " +
		"acpid"
	_, prereqErr := shared.RunCommandOnNode(installPrereqs, ip)
	Expect(prereqErr).ToNot(HaveOccurred(), "error installing prerequisites: %v", prereqErr)
	shared.LogLevel("info", "Installed prerequisite packages")

	driverVersion := "570.133.20"
	downloadDriver := "sudo curl -fSsl -O  https://us.download.nvidia.com/tesla/" + driverVersion + "/NVIDIA-Linux-x86_64-" +
		driverVersion + ".run"
	_, downloadErr := shared.RunCommandOnNode(downloadDriver, ip)
	Expect(downloadErr).ToNot(HaveOccurred(), "error downloading NVIDIA driver: %v", downloadErr)
	shared.LogLevel("info", "Downloaded NVIDIA driver version %s", driverVersion)

	kernelVersion := "$(uname -r)"
	modulesPath := "/lib/modules/" + kernelVersion + "/build"
	driverInstall := "sudo bash NVIDIA-Linux-x86_64-" + driverVersion + ".run --accept-license --silent --no-questions " +
		" --ui=none --kernel-source-path=" + modulesPath
	_, installErr := shared.RunCommandOnNode(driverInstall, ip)
	Expect(installErr).ToNot(HaveOccurred(), "error installing NVIDIA driver: %v", installErr)

	shared.LogLevel("info", "Installed NVIDIA driver")
}

func initialSetupSles(ip string) {
	driverVersion := "570.124.06"
	installDriver := "sudo zypper -v --non-interactive in 'nvidia-open-driver-G06-signed-cuda-kmp== " + driverVersion + "' "
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
	// create a empty dummy repo file to GPU operator acknowledge.
	repoFile := "sudo mkdir -p /etc/yum.repos.d && sudo touch /etc/yum.repos.d/redhat.repo && " +
		"sudo chmod 644 /etc/yum.repos.d/redhat.repo"
	_, repoErr := shared.RunCommandOnNode(repoFile, ip)
	Expect(repoErr).ToNot(HaveOccurred(), "error creating repo file: %v", repoErr)

	driverVersion := "570.133.20"
	downloadDriver := "sudo curl -fSsl -O  https://us.download.nvidia.com/tesla/" +
		driverVersion + "/NVIDIA-Linux-x86_64-" + driverVersion + ".run"
	_, downloadErr := shared.RunCommandOnNode(downloadDriver, ip)
	Expect(downloadErr).ToNot(HaveOccurred(), "error downloading NVIDIA driver: %v", downloadErr)
	shared.LogLevel("info", "Downloaded NVIDIA driver version %s", driverVersion)

	checkKernel := "uname -r"
	kernelVersion, kernelCheckErr := shared.RunCommandOnNode(checkKernel, ip)
	Expect(kernelCheckErr).ToNot(HaveOccurred(), "error checking kernel version: %v", kernelCheckErr)
	kernelVersion = strings.TrimSpace(kernelVersion)
	shared.LogLevel("info", "Kernel version: %s", kernelVersion)

	kernelPackages := fmt.Sprintf("sudo yum -y install kernel-devel-%s kernel-headers-%s gcc make acpid pkg-config ",
		kernelVersion, kernelVersion)
	_, kernelErr := shared.RunCommandOnNode(kernelPackages, ip)
	Expect(kernelErr).ToNot(HaveOccurred(), "error installing kernel packages: %v", kernelErr)
	shared.LogLevel("info", "Installed kernel development packages")

	// update sim link so when driver is installed, it will use the correct kernel version and it will find the path.
	kernelPath := "/usr/src/kernels/" + kernelVersion
	kernelSimLinkPath := "/usr/lib/modules/" + kernelVersion + "/build"
	sl := "sudo ln -sf " + kernelPath + " " + kernelSimLinkPath
	_, slErr := shared.RunCommandOnNode(sl, ip)
	Expect(slErr).ToNot(HaveOccurred(), "error creating symlink: %v", slErr)

	driverInstall := "sudo bash NVIDIA-Linux-x86_64-" + driverVersion + ".run --accept-license --silent --no-questions" +
		" --ui=none --kernel-source-path=" + kernelSimLinkPath
	_, installErr := shared.RunCommandOnNode(driverInstall, ip)
	Expect(installErr).ToNot(HaveOccurred(), "error installing NVIDIA driver: %v", installErr)
	shared.LogLevel("info", "Installed NVIDIA driver")

	s := "sudo setenforce 0"
	_, cmdErr := shared.RunCommandOnNode(s, ip)
	Expect(cmdErr).ToNot(HaveOccurred(), "error setting SELinux to permissive mode: %v", cmdErr)
}

func validateNvidiaVersion(ip string) {
	versionCmd := "sudo cat /proc/driver/nvidia/version"

	cfg := shared.CmdNodeRetryCfg()
	cfg.Attempts = 20
	cfg.Delay = 10 * time.Second
	cfg.RetryableErrorSubString = []string{
		"No such file or directory",
	}

	res, err := shared.RunCommandOnNodeWithRetry(versionCmd, ip, &cfg)
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
	cmd := fmt.Sprintf("kubectl get node %s  --kubeconfig=%s -o jsonpath=\"{.metadata.labels}\" ",
		strings.TrimSpace(nodeName), shared.KubeConfigFile)

	labelsToFind := []string{
		"\"nvidia.com/gpu.deploy.driver\":" + "\"pre-installed\"",
		"nvidia.com/cuda.driver.major",
		"nvidia.com/gpu.machine",
		"nvidia.com/gpu.count",
		"nvidia.com/gpu.product",
	}

	retryErr := retry.Do(
		func() error {
			res, err := shared.RunCommandHost(cmd)
			if err != nil {
				return fmt.Errorf("failed to get node labels: %w", err)
			}

			if strings.TrimSpace(res) == "" {
				return errors.New("node labels are empty")
			}

			for _, label := range labelsToFind {
				if !strings.Contains(res, label) {
					return fmt.Errorf("label %s not found in node labels", label)
				}
			}

			return nil
		},
		retry.Attempts(20),
		retry.Delay(10*time.Second),
		retry.OnRetry(func(n uint, err error) {
			shared.LogLevel("warn", "Attempt %d failed, retrying to get node labels: %v", n+1, err)
		}))

	Expect(retryErr).NotTo(HaveOccurred(), "failed to get node labels after multiple attempts: %v", retryErr)
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

func validateNvidiaModule(ip string) error {
	lsmodCmd := "sudo lsmod | grep nvidia"

	modulesToCheck := []string{
		"nvidia",
		"nvidia_uvm",
		"nvidia_drm",
		"nvidia_modeset",
	}

	cfg := shared.CmdNodeRetryCfg()
	cfg.Attempts = 20
	cfg.Delay = 10 * time.Second

	out, err := shared.RunCommandOnNodeWithRetry(lsmodCmd, ip, &cfg)
	Expect(err).NotTo(HaveOccurred(),
		"failed to find nvidia module via lsmod after multiple attempts: %v", err)

	for _, module := range modulesToCheck {
		if !strings.Contains(out, module) {
			shared.LogLevel("warn", "NVIDIA module %s not found in lsmod output:\n%s\nRetrying...", module, out)

			output, retryErr := shared.RunCommandOnNode(module, ip)
			if !strings.Contains(output, module) {
				return fmt.Errorf("NVIDIA module %s not found in lsmod output:\n%s\n%v", module, output, retryErr)
			}
		}
	}

	shared.LogLevel("info", "NVIDIA modules found:\n%s", out)

	return nil
}

func validateNvidiaBenchmarkPodStatus() {
	cmd := fmt.Sprintf("kubectl get pod nbody-gpu-benchmark -n kube-system --kubeconfig=%s -o jsonpath='{.status.phase}'",
		shared.KubeConfigFile)

	var podStatus string
	var err error
	retryErr := retry.Do(
		func() error {
			podStatus, err = shared.RunCommandHost(cmd)
			if err != nil {
				return fmt.Errorf("failed to get benchmark pod status: %w", err)
			}

			podStatus = strings.TrimSpace(podStatus)
			if podStatus != "Succeeded" && podStatus != "Running" && podStatus != "Completed" {
				return errors.New("benchmark pod status is not Succeeded/Running/Completed")
			}

			return nil
		},
		retry.Attempts(20),
		retry.Delay(5*time.Second),
		retry.OnRetry(func(n uint, err error) {
			shared.LogLevel("warn", "Attempt %d failed, retrying to get benchmark pod status: %v", n+1, err)
		}),
	)
	Expect(retryErr).NotTo(HaveOccurred(), "failed to get benchmark pod status after multiple attempts: %v", retryErr)

	shared.LogLevel("info", "Benchmark pod status: %s", podStatus)
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
