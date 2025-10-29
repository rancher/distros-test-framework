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

func TestNvidiaGPUFunctionality(cluster *shared.Cluster, nvidiaVersion string) {
	// for now we are only testing integration with the first server in the cluster.
	targetNodeIP := cluster.ServerIPs[0]
	nodeOs = cluster.NodeOS

	verifyGPUHardwarePresence(targetNodeIP)

	switch nodeOs {
	case "ubuntu":
		shared.LogLevel("info", "Proceeding with Ubuntu setup for NVIDIA driver installation version: %s", nvidiaVersion)
		initialSetupUbuntu(targetNodeIP, nvidiaVersion)
	case "rhel", "rhel8", "rhel9":
		shared.LogLevel("info", "Proceeding with RHEL setup for NVIDIA driver installation version: %s", nvidiaVersion)
		initialSetupRHEL(targetNodeIP, nvidiaVersion)
	case "sles15":
		shared.LogLevel("info", "Proceeding with SLES setup for NVIDIA driver installation with latest available driver")
		initialSetupSles(targetNodeIP)
	default:
		shared.LogLevel("error", "Unsupported OS: %s version: %s", nodeOs, nvidiaVersion)
		return
	}

	rebootNode(targetNodeIP)

	validateNvidiaVersion(targetNodeIP)
	validateNvidiaLibMl(targetNodeIP)

	workloadErr := shared.ManageWorkload("apply", "nvidia-operator.yaml")
	Expect(workloadErr).NotTo(HaveOccurred(), "nvidia operator manifests not deployed")

	shared.LogLevel("info", "Waiting needed as per documentation for operator to restart containerd and stabilize")
	time.Sleep(60 * time.Second)

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

func verifyGPUHardwarePresence(ip string) {
	shared.LogLevel("info", "Verifying NVIDIA GPU hardware is present")
	checkGPU := "lspci | grep -i nvidia || echo 'NO_GPU_FOUND'"
	gpuCheck, gpuCheckErr := shared.RunCommandOnNode(checkGPU, ip)
	Expect(gpuCheckErr).ToNot(HaveOccurred(), "error checking GPU hardware: %v", gpuCheckErr)
	Expect(gpuCheck).To(ContainSubstring("NVIDIA"),
		"No NVIDIA GPU hardware found. This test requires a GPU-enabled EC2 instance.")

	shared.LogLevel("info", "NVIDIA GPU hardware detected:\n%s", strings.TrimSpace(gpuCheck))
}

func initialSetupUbuntu(ip, nvidiaVersion string) {
	Expect(nvidiaVersion).NotTo(BeEmpty(), "nvidiaVersion parameter is required for Ubuntu. "+
		"Please set NVIDIA_VERSION environment variable or pass it as a flag to the test.")

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

	shared.LogLevel("info", "Downloading NVIDIA driver version %s from NVIDIA website", nvidiaVersion)
	downloadDriver := "sudo curl -fSsl -O  https://us.download.nvidia.com/tesla/" + nvidiaVersion + "/NVIDIA-Linux-x86_64-" +
		nvidiaVersion + ".run"
	_, downloadErr := shared.RunCommandOnNode(downloadDriver, ip)
	Expect(downloadErr).ToNot(HaveOccurred(), "error downloading NVIDIA driver: %v", downloadErr)
	shared.LogLevel("info", "Downloaded NVIDIA driver version %s", nvidiaVersion)

	kernelVersion := "$(uname -r)"
	modulesPath := "/lib/modules/" + kernelVersion + "/build"
	driverInstall := "sudo bash NVIDIA-Linux-x86_64-" + nvidiaVersion + ".run --accept-license --silent --no-questions " +
		" --ui=none --kernel-source-path=" + modulesPath
	_, installErr := shared.RunCommandOnNode(driverInstall, ip)
	Expect(installErr).ToNot(HaveOccurred(), "error installing NVIDIA driver: %v", installErr)

	shared.LogLevel("info", "Installed NVIDIA driver")
}

func rebootNode(targetNodeIP string) {
	shared.LogLevel("info", "Rebooting node to ensure NVIDIA driver is properly loaded")
	rebootCmd := "sudo reboot"
	_, _ = shared.RunCommandOnNode(rebootCmd, targetNodeIP)

	shared.LogLevel("info", "Waiting for node to reboot (60 seconds)...")
	time.Sleep(60 * time.Second)

	shared.LogLevel("info", "Waiting for node to come back online...")
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		pingCmd := "echo 'ping'"
		_, pingErr := shared.RunCommandOnNode(pingCmd, targetNodeIP)
		if pingErr == nil {
			shared.LogLevel("info", "Node is back online")
			break
		}
		if i == maxRetries-1 {
			Expect(pingErr).ToNot(HaveOccurred(), "Node did not come back online after reboot")
		}
		time.Sleep(10 * time.Second)
	}

	shared.LogLevel("info", "Waiting for RKE2 to be fully ready after reboot...")
	for i := 0; i < 30; i++ {
		checkRke2 := "systemctl is-active rke2-server"
		rke2Status, _ := shared.RunCommandOnNode(checkRke2, targetNodeIP)
		if strings.TrimSpace(rke2Status) == "active" {
			shared.LogLevel("info", "RKE2 is active, waiting additional 30s for core pods...")
			time.Sleep(30 * time.Second)
			break
		}
		if i == 29 {
			shared.LogLevel("warn", "RKE2 may not be fully ready, proceeding anyway")
		}
		time.Sleep(10 * time.Second)
	}
}

func initialSetupSles(ip string) {
	ensureSlEsRegistration(ip)
	installNvidiaDriverSles(ip)
	installNvidiaComputeUtilsSles(ip)
}

func ensureSlEsRegistration(ip string) {
	// check for zypper locks and clear them if needed.
	clearLocks := "sudo pkill -f zypper 2>/dev/null || true; sudo rm -f /var/run/zypp.pid 2>/dev/null || true; sleep 2"
	_, _ = shared.RunCommandOnNode(clearLocks, ip)

	shared.LogLevel("info", "Checking SLES registration status")
	checkRepos := "sudo zypper lr 2>&1"
	repoStatus, _ := shared.RunCommandOnNode(checkRepos, ip)
	if strings.Contains(repoStatus, "No repositories defined") || strings.Contains(repoStatus, "Warning: No repositories") {
		shared.LogLevel("warn", "No repositories configured, attempting cloud registration")
		registerCmd := "sudo registercloudguest --force-new 2>&1"
		regRes, regErr := shared.RunCommandOnNode(registerCmd, ip)
		shared.LogLevel("debug", "Registration output:\n%s", regRes)

		if regErr != nil || !strings.Contains(regRes, "succeeded") {
			shared.LogLevel("error", "Failed to register with cloud update server: %v", regErr)
			Expect(regErr).ToNot(HaveOccurred(),
				"SLES registration failed, cannot install NVIDIA driver without repos: %v\nOutput: %s", regErr, regRes)
		}
		shared.LogLevel("info", "SLES registration successful")
	} else {
		shared.LogLevel("info", "SLES repositories already configured")
	}
}

func installNvidiaDriverSles(ip string) {
	// get the current kernel variant to match the correct kmp package.
	getKernel := "uname -r | awk -F'-' '{print $NF}'"
	kernelVariant, kernelErr := shared.RunCommandOnNode(getKernel, ip)
	if kernelErr != nil {
		shared.LogLevel("warn", "Failed to detect kernel variant, defaulting to 'default': %v", kernelErr)
		kernelVariant = "default"
	}
	kernelVariant = strings.TrimSpace(kernelVariant)
	shared.LogLevel("info", "Detected kernel variant: %s", kernelVariant)
	driverPackage := "nvidia-open-driver-G06-signed-cuda-kmp-" + kernelVariant

	// always install latest available - SLES manages driver versions in repos.
	installDriver := "sudo zypper -v --non-interactive in " + driverPackage + " 2>&1"
	res, installDriverErr := shared.RunCommandOnNode(installDriver, ip)
	shared.LogLevel("debug", "Driver installation output:\n%s", res)
	Expect(installDriverErr).ToNot(HaveOccurred(), "error installing driver: %v\nOutput: %s", installDriverErr, res)

	checkInstalled := "rpm -q " + driverPackage
	installedVer, _ := shared.RunCommandOnNode(checkInstalled, ip)
	shared.LogLevel("info", "Installed NVIDIA driver: %s", strings.TrimSpace(installedVer))

	shared.LogLevel("info", "Loading NVIDIA kernel module")
	loadModule := "sudo modprobe nvidia && sudo modprobe nvidia-uvm"
	modRes, modErr := shared.RunCommandOnNode(loadModule, ip)
	if modErr != nil {
		shared.LogLevel("warn", "Failed to load NVIDIA module: %v, output: %s", modErr, modRes)
		shared.LogLevel("info", "Checking dmesg for kernel module errors")
		dmesgCheck := "sudo dmesg | grep -i nvidia | tail -20"
		dmesgOut, _ := shared.RunCommandOnNode(dmesgCheck, ip)
		shared.LogLevel("debug", "dmesg output:\n%s", dmesgOut)
		Expect(modErr).ToNot(HaveOccurred(), "error loading NVIDIA kernel module: %v\nOutput: %s", modErr, modRes)
	}

	verifyModule := "lsmod | grep nvidia"
	modCheck, modCheckErr := shared.RunCommandOnNode(verifyModule, ip)
	Expect(modCheckErr).ToNot(HaveOccurred(), "NVIDIA module not loaded: %v", modCheckErr)
	shared.LogLevel("info", "NVIDIA kernel modules loaded:\n%s", strings.TrimSpace(modCheck))
}

func installNvidiaComputeUtilsSles(ip string) {
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

	checkNvidiaSmi := "which nvidia-smi"
	_, nvidiaSmiErr := shared.RunCommandOnNode(checkNvidiaSmi, ip)
	if nvidiaSmiErr == nil {
		shared.LogLevel("info", "nvidia-smi already available, skipping compute-utils installation")
		return
	}

	installComputeUtils := "sudo zypper -v --non-interactive in -r cuda nvidia-compute-utils-G06 2>&1"
	res, installComputeUtilsErr := shared.RunCommandOnNode(installComputeUtils, ip)
	if installComputeUtilsErr != nil {
		shared.LogLevel("error", "Failed to install nvidia-compute-utils-G06 "+
			"from CUDA repo: %v %v", installComputeUtilsErr, res)
		return
	}

	_, finalCheck := shared.RunCommandOnNode(checkNvidiaSmi, ip)
	Expect(finalCheck).ToNot(HaveOccurred(), "nvidia-smi not found after compute-utils installation")
	shared.LogLevel("info", "Successfully installed NVIDIA compute utils")
}

func initialSetupRHEL(ip, nvidiaVersion string) {
	Expect(nvidiaVersion).NotTo(BeEmpty(), "nvidiaVersion parameter is required for RHEL. "+
		"Please set NVIDIA_VERSION environment variable or pass it as a flag to the test.")

	// create a empty dummy repo file to GPU operator acknowledge.
	repoFile := "sudo mkdir -p /etc/yum.repos.d && sudo touch /etc/yum.repos.d/redhat.repo && " +
		"sudo chmod 644 /etc/yum.repos.d/redhat.repo"
	_, repoErr := shared.RunCommandOnNode(repoFile, ip)
	Expect(repoErr).ToNot(HaveOccurred(), "error creating repo file: %v", repoErr)

	shared.LogLevel("info", "Downloading NVIDIA driver version %s from NVIDIA website", nvidiaVersion)
	downloadDriver := "sudo curl -fSsl -O  https://us.download.nvidia.com/tesla/" +
		nvidiaVersion + "/NVIDIA-Linux-x86_64-" + nvidiaVersion + ".run"
	_, downloadErr := shared.RunCommandOnNode(downloadDriver, ip)
	Expect(downloadErr).ToNot(HaveOccurred(), "error downloading NVIDIA driver: %v", downloadErr)
	shared.LogLevel("info", "Downloaded NVIDIA driver version %s", nvidiaVersion)

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

	// update sim link so when driver is installed,
	// it will use the correct kernel version and it will find the path.
	kernelPath := "/usr/src/kernels/" + kernelVersion
	kernelSimLinkPath := "/usr/lib/modules/" + kernelVersion + "/build"
	sl := "sudo ln -sf " + kernelPath + " " + kernelSimLinkPath
	_, slErr := shared.RunCommandOnNode(sl, ip)
	Expect(slErr).ToNot(HaveOccurred(), "error creating symlink: %v", slErr)

	driverInstall := "sudo bash NVIDIA-Linux-x86_64-" + nvidiaVersion + ".run --accept-license --silent --no-questions" +
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
	// search for libnvidia-ml library (may have version suffix like .so.1 or .so.580.95.05)
	findCmd := "sudo find /usr/ -name 'libnvidia-ml.so*' 2>/dev/null | head -5"

	res, err := shared.RunCommandOnNode(findCmd, ip)
	Expect(err).NotTo(HaveOccurred(), "failed to find libnvidia-ml.so: %v", err)
	Expect(res).NotTo(BeEmpty(), "libnvidia-ml.so library not found in /usr/")
	Expect(res).To(Or(
		ContainSubstring("/usr/lib64/libnvidia-ml.so"),
		ContainSubstring("/usr/lib/x86_64-linux-gnu/libnvidia-ml.so")),
		"libnvidia-ml.so not found in expected library paths")

	shared.LogLevel("info", "libnvidia-ml.so library found:\n%s", res)
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
	cmd := fmt.Sprintf("kubectl get pod nbody-gpu-benchmark -n test-nvidia-benchmark "+
		"--kubeconfig=%s -o jsonpath='{.status.phase}'",
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
	Expect(retryErr).NotTo(HaveOccurred(), "failed to get benchmark pod status "+
		"after multiple attempts: %v", retryErr)

	shared.LogLevel("info", "Benchmark pod status: %s", podStatus)
}

func validateBenchmark() {
	benchmarkLogs := "kubectl logs nbody-gpu-benchmark -n test-nvidia-benchmark " +
		"--kubeconfig=" + shared.KubeConfigFile
	logs, logErr := shared.RunCommandHost(benchmarkLogs)
	Expect(logErr).NotTo(HaveOccurred(), "Failed to get benchmark pod logs")

	Expect(logs).To(ContainSubstring(" CUDA device: [Tesla T4]"),
		"CUDA device not found in benchmark logs")
	Expect(logs).To(ContainSubstring("billion interactions per second"),
		"Benchmark logs did not contain 'billion interactions per second info'")

	shared.LogLevel("info", "Benchmark logs contain nvidia device and performance info:\n%s", logs)
}

// Tests below are pulled from - https://docs.google.com/document/d/1zy0key-EL6JH50MZgwg96RPYxxXXnVUdxLZwGiyqLd8/

func TestNvidiaUnprivilegedPod() {
	validatePodWithGPULimits()
	validatePodWithGPULimitsAndEnvVars()
	validatePodWithoutGPULimitsNoEnvVar()
	validatePodWithoutGPULimitsWithEnvVar()
	validatePodWithoutGPULimitsEmptyEnvVar()
}

// validatePodWithGPULimits tests an unprivileged pod requesting GPUs.
// GPU limits are set to 1, NVIDIA_VISIBLE_DEVICES is unset.
// Expected: Should succeed and nvidia-smi should list GPUs.
func validatePodWithGPULimits() {
	podName := "nvidia-test-pod-gpu-limits"
	image := "nvcr.io/nvidia/cuda:9.0-base"
	command := "nvidia-smi -L"

	err := shared.CleanupPod(podName)
	Expect(err).NotTo(HaveOccurred(), "failed to cleanup pod: %v", err)

	res, err := runPodWithGPU(podName, image, command, 1, nil, false)
	Expect(err).NotTo(HaveOccurred(), "failed to run pod with GPU limits: %v", err)

	Expect(res).To(ContainSubstring("GPU"), "nvidia-smi output should contain GPU information")

	shared.LogLevel("info", "Unprivileged pod with GPU limits succeeded:\n%s", res)
}

// validatePodWithGPULimitsAndEnvVars tests an unprivileged pod requesting GPUs with env vars.
// GPU limits are set to 1, NVIDIA_VISIBLE_DEVICES is set to "all".
// Expected: Should succeed and environment variables should be set correctly.
func validatePodWithGPULimitsAndEnvVars() {
	podName := "nvidia-test-pod-gpu-env"
	image := "nvcr.io/nvidia/cuda:9.0-base"
	command := "export"

	err := shared.CleanupPod(podName)
	Expect(err).NotTo(HaveOccurred(), "failed to cleanup pod: %v", err)

	envVars := map[string]string{
		"NVIDIA_VISIBLE_DEVICES": "all",
	}

	res, err := runPodWithGPU(podName, image, command, 1, envVars, false)
	Expect(err).NotTo(HaveOccurred(), "failed to run pod with GPU limits and env vars: %v", err)
	Expect(res).To(ContainSubstring("NVIDIA_VISIBLE_DEVICES"),
		"environment variables should contain NVIDIA_VISIBLE_DEVICES")
	Expect(res).To(ContainSubstring("NVIDIA_DRIVER_CAPABILITIES"),
		"environment variables should contain NVIDIA_DRIVER_CAPABILITIES")
	Expect(res).To(ContainSubstring("LD_LIBRARY_PATH"),
		"environment variables should contain LD_LIBRARY_PATH")

	shared.LogLevel("info", "Unprivileged pod with GPU limits and env vars succeeded:\n%s", res)
}

// validatePodWithoutGPULimitsNoEnvVar tests an unprivileged pod NOT requesting GPUs.
// GPU limits are 0, NVIDIA_VISIBLE_DEVICES is unset.
// Expected: Should fail with insufficient privileges or command not found error.
func validatePodWithoutGPULimitsNoEnvVar() {
	podName := "nvidia-test-pod-no-gpu"
	image := "nvcr.io/nvidia/cuda:9.0-base"
	command := "nvidia-smi -L"

	err := shared.CleanupPod(podName)
	Expect(err).NotTo(HaveOccurred(), "failed to cleanup pod: %v", err)

	res, err := runPodWithGPU(podName, image, command, 0, nil, false)
	Expect(err).To(HaveOccurred(), "pod without GPU limits should fail")
	Expect(res).To(Or(
		ContainSubstring("insufficient privileges"),
		ContainSubstring("StartError"),
		ContainSubstring("failed to create containerd task"),
		ContainSubstring("terminated (Error)"),
		ContainSubstring("exit status 127"),
	), "expected error (insufficient privileges or command not found), got: %s", res)

	shared.LogLevel("info", "Unprivileged pod without GPU limits failed as expected:\n%s", res)
}

// validatePodWithoutGPULimitsWithEnvVar tests an unprivileged pod NOT requesting GPUs.
// GPU limits are 0, NVIDIA_VISIBLE_DEVICES is set to "0".
// Expected: Should fail with insufficient privileges or command not found error.
func validatePodWithoutGPULimitsWithEnvVar() {
	podName := "nvidia-test-pod-no-gpu-env"
	image := "nvcr.io/nvidia/cuda:9.0-base"
	command := "nvidia-smi -L"

	err := shared.CleanupPod(podName)
	Expect(err).NotTo(HaveOccurred(), "failed to cleanup pod: %v", err)

	envVars := map[string]string{
		"NVIDIA_VISIBLE_DEVICES": "0",
	}

	res, err := runPodWithGPU(podName, image, command, 0, envVars, false)
	Expect(err).To(HaveOccurred(), "pod without GPU limits but with env var should fail")
	Expect(res).To(Or(
		ContainSubstring("insufficient privileges"),
		ContainSubstring("StartError"),
		ContainSubstring("failed to create containerd task"),
		ContainSubstring("terminated (Error)"),
		ContainSubstring("exit status 127"),
	), "expected error (insufficient privileges or command not found), got: %s", res)

	shared.LogLevel("info", "Unprivileged pod without GPU limits but with env var failed as expected:\n%s", res)
}

// validatePodWithoutGPULimitsEmptyEnvVar tests an unprivileged pod NOT requesting GPUs.
// GPU limits are 0, NVIDIA_VISIBLE_DEVICES is set to empty string "".
// Expected: Should fail with nvidia-smi not found error because GPU access is denied.
func validatePodWithoutGPULimitsEmptyEnvVar() {
	podName := "nvidia-test-pod-empty-env"
	image := "nvcr.io/nvidia/cuda:9.0-base"
	command := "nvidia-smi -L"

	err := shared.CleanupPod(podName)
	Expect(err).NotTo(HaveOccurred(), "failed to cleanup pod: %v", err)

	envVars := map[string]string{
		"NVIDIA_VISIBLE_DEVICES": "",
	}

	res, err := runPodWithGPU(podName, image, command, 0, envVars, false)
	Expect(err).To(HaveOccurred(), "pod with empty NVIDIA_VISIBLE_DEVICES should fail")
	Expect(res).To(Or(
		ContainSubstring("executable file not found"),
		ContainSubstring("nvidia-smi"),
		ContainSubstring("failed to create containerd task"),
		ContainSubstring("terminated (Error)"),
		ContainSubstring("exit status 127"),
	), "expected nvidia-smi not found error, got: %s", res)

	shared.LogLevel("info", "Unprivileged pod with empty env var failed as expected:\n%s", res)

	// verify environment variables are still set correctly.
	res, err = runPodWithGPU(podName, image, "export", 0, envVars, false)
	Expect(err).NotTo(HaveOccurred(), "should be able to run export command")
	Expect(res).To(Or(
		ContainSubstring("NVIDIA_VISIBLE_DEVICES=\"\""),
		ContainSubstring("NVIDIA_VISIBLE_DEVICES=''")),
		"NVIDIA_VISIBLE_DEVICES should be empty string")

	shared.LogLevel("info", "Environment variables verified:\n%s", res)
}

// TestNvidiaPrivilegedPod tests a privileged pod NOT requesting GPUs.
// GPU limits are 0, NVIDIA_VISIBLE_DEVICES is set to "0", privileged is true.
// Expected: Should succeed even without GPU limits due to privileged access.
func TestNvidiaPrivilegedPod() {
	podName := "nvidia-test-pod-privileged"
	image := "nvcr.io/nvidia/cuda:9.0-base"
	command := "nvidia-smi -L"

	err := shared.CleanupPod(podName)
	Expect(err).NotTo(HaveOccurred(), "failed to cleanup pod: %v", err)

	envVars := map[string]string{
		"NVIDIA_VISIBLE_DEVICES": "0",
	}

	res, err := runPodWithGPU(podName, image, command, 0, envVars, true)
	Expect(err).NotTo(HaveOccurred(), "failed to run privileged pod without GPU limits: %v\n%s", err, res)
	Expect(res).To(ContainSubstring("GPU"), "nvidia-smi output should contain GPU information")

	shared.LogLevel("info", "Privileged pod without GPU limits succeeded:\n%s", res)
}

func runPodWithGPU(
	podName, image, command string,
	gpuLimit int,
	envVars map[string]string,
	privileged bool,
) (string, error) {
	var envJSON string
	if len(envVars) > 0 {
		var envItems []string
		for key, value := range envVars {
			envItems = append(envItems, fmt.Sprintf(`{"name":"%s","value":"%s"}`, key, value))
		}
		envJSON = fmt.Sprintf(`"env":[%s],`, strings.Join(envItems, ","))
	}

	var limitsJSON string
	if gpuLimit > 0 {
		limitsJSON = fmt.Sprintf(`"resources":{"limits":{"nvidia.com/gpu":"%d"}},`, gpuLimit)
	}

	var securityJSON string
	if privileged {
		securityJSON = `"securityContext":{"privileged":true},`
	}

	overrides := fmt.Sprintf(`{
		"spec":{
			"runtimeClassName":"nvidia",
			"containers":[{
				"name":"%s",
				"image":"%s",
				"command":["sh","-c","%s"],
				%s%s%s
				"stdin":true,
				"tty":false
			}],
			"restartPolicy":"Never"
		}
	}`, podName, image, command, envJSON, limitsJSON, securityJSON)

	overrides = strings.ReplaceAll(overrides, "\n", "")
	overrides = strings.ReplaceAll(overrides, "\t", "")

	cmd := fmt.Sprintf("kubectl run %s --image=%s --restart=Never --rm -i --overrides='%s' --kubeconfig=%s",
		podName, image, overrides, shared.KubeConfigFile)

	return shared.RunCommandHost(cmd)
}
