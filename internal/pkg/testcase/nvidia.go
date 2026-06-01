package testcase

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go"

	. "github.com/onsi/gomega"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

var nodeOs string

func TestNvidiaGPUFunctionality(cluster *driver.Cluster, nvidiaVersion string) {
	// for now we are only testing integration with the first server in the cluster.
	targetNodeIP := cluster.ServerIPs[0]
	nodeOs = cluster.NodeOS

	verifyGPUHardwarePresence(targetNodeIP, nodeOs)

	switch nodeOs {
	case "ubuntu":
		resources.LogLevel("info", "Proceeding with Ubuntu setup for NVIDIA driver installation version: %s", nvidiaVersion)
		initialSetupUbuntu(targetNodeIP, nvidiaVersion)
	case "rhel", "rhel8", "rhel9":
		resources.LogLevel("info", "Proceeding with RHEL setup for NVIDIA driver installation version: %s", nvidiaVersion)
		initialSetupRHEL(targetNodeIP, nvidiaVersion)
	case "sles15":
		resources.LogLevel("info", "Proceeding with SLES setup for NVIDIA driver installation with latest available driver")
		initialSetupSles(targetNodeIP)
	default:
		resources.LogLevel("error", "Unsupported OS: %s version: %s", nodeOs, nvidiaVersion)
		return
	}

	validateNvidiaVersion(targetNodeIP)
	validateNvidiaLibMl(targetNodeIP)

	workloadErr := resources.ManageWorkload("apply", "nvidia-operator.yaml")
	Expect(workloadErr).NotTo(HaveOccurred(), "nvidia operator manifests not deployed")

	resources.LogLevel("info", "Waiting needed as per documentation for operator to restart containerd and stabilize")
	time.Sleep(60 * time.Second)

	nodeName, err := resources.RunCommandHost("kubectl get nodes -o jsonpath='{.items[0].metadata.name}' " +
		"--kubeconfig=" + resources.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(), "failed to get node name: %v", err)
	Expect(nodeName).NotTo(BeEmpty(), "Node name is empty")

	validateNvidiaOperatorDeploy(nodeName)

	validateNvidiaGPU(nodeName)

	validateNvidiaRunBinPath(targetNodeIP)

	validateContainerdConfig(targetNodeIP)

	validateNvidiaToolKit(targetNodeIP)

	err = validateNvidiaModule(targetNodeIP)
	Expect(err).NotTo(HaveOccurred(), "NVIDIA module not found: %v", err)

	workloadErr = resources.ManageWorkload("apply", "nvidia-benchmark.yaml")
	Expect(workloadErr).NotTo(HaveOccurred(), "nvidia benchmark manifests not deployed")
	validateNvidiaBenchmarkPodStatus()
	validateBenchmark()
}

func verifyGPUHardwarePresence(ip, nodeOs string) {
	resources.LogLevel("info", "Verifying NVIDIA GPU hardware is present")

	ensureLspciInstalled(ip, nodeOs)

	checkGPU := "lspci | grep -i nvidia || echo 'NO_GPU_FOUND'"
	gpuCheck, gpuCheckErr := resources.RunCommandOnNode(checkGPU, ip)
	Expect(gpuCheckErr).ToNot(HaveOccurred(), "error checking GPU hardware: %v", gpuCheckErr)
	Expect(gpuCheck).To(ContainSubstring("NVIDIA"),
		"No NVIDIA GPU hardware found. This test requires a GPU-enabled EC2 instance.")

	resources.LogLevel("info", "NVIDIA GPU hardware detected:\n%s", strings.TrimSpace(gpuCheck))
}

func ensureLspciInstalled(ip, nodeOs string) {
	checkCmd := "command -v lspci > /dev/null 2>&1 && echo 'AMHEREALREADY' || echo 'NOTHERE'"
	result, err := resources.RunCommandOnNode(checkCmd, ip)
	Expect(err).ToNot(HaveOccurred(), "error checking for lspci: %v", err)

	if strings.Contains(result, "AMHEREALREADY") {
		resources.LogLevel("info", "lspci is already available")
		return
	}

	resources.LogLevel("info", "lspci not found, installing pciutils package for OS: %s", nodeOs)
	var installCmd string
	switch nodeOs {
	case "ubuntu":
		installCmd = "sudo apt update && sudo DEBIAN_FRONTEND=noninteractive apt install -y pciutils"
	case "rhel", "rhel8", "rhel9":
		installCmd = "sudo yum install -y pciutils || sudo dnf install -y pciutils"
	case "sles15":
		installCmd = "sudo zypper install -y pciutils"
	default:
		resources.LogLevel("warn", "Unknown OS %s, attempting yum/dnf install", nodeOs)
		installCmd = "sudo yum install -y pciutils || sudo dnf install -y pciutils || sudo apt install -y pciutils"
	}

	_, installErr := resources.RunCommandOnNode(installCmd, ip)
	Expect(installErr).ToNot(HaveOccurred(), "error installing pciutils: %v", installErr)
	resources.LogLevel("info", "pciutils installed successfully")
}

func initialSetupUbuntu(ip, nvidiaVersion string) {
	Expect(nvidiaVersion).NotTo(BeEmpty(), "nvidiaVersion parameter is required for Ubuntu. "+
		"Please set NVIDIA_VERSION environment variable or pass it as a flag to the test.")

	updateCmd := "sudo apt update"
	_, updateErr := resources.RunCommandOnNode(updateCmd, ip)
	Expect(updateErr).ToNot(HaveOccurred(), "error updating package lists: %v", updateErr)
	resources.LogLevel("info", "Updated package lists")

	installPrereqs := "DEBIAN_FRONTEND=noninteractive sudo apt install -y build-essential " +
		"linux-headers-$(uname -r) " +
		"pkg-config " +
		"libglvnd-dev " +
		"xorg-dev " +
		"vulkan-tools " +
		"dkms " +
		"acpid"
	_, prereqErr := resources.RunCommandOnNode(installPrereqs, ip)
	Expect(prereqErr).ToNot(HaveOccurred(), "error installing prerequisites: %v", prereqErr)
	resources.LogLevel("info", "Installed prerequisite packages")

	resources.LogLevel("info", "Downloading NVIDIA driver version %s from NVIDIA website", nvidiaVersion)
	downloadDriver := "sudo curl -fSsl -O  https://us.download.nvidia.com/tesla/" + nvidiaVersion + "/NVIDIA-Linux-x86_64-" +
		nvidiaVersion + ".run"
	_, downloadErr := resources.RunCommandOnNode(downloadDriver, ip)
	Expect(downloadErr).ToNot(HaveOccurred(), "error downloading NVIDIA driver: %v", downloadErr)
	resources.LogLevel("info", "Downloaded NVIDIA driver version %s", nvidiaVersion)

	kernelVersion := "$(uname -r)"
	modulesPath := "/lib/modules/" + kernelVersion + "/build"
	driverInstall := "sudo bash NVIDIA-Linux-x86_64-" + nvidiaVersion + ".run --accept-license --silent --no-questions " +
		" --ui=none --kernel-source-path=" + modulesPath
	_, installErr := resources.RunCommandOnNode(driverInstall, ip)
	Expect(installErr).ToNot(HaveOccurred(), "error installing NVIDIA driver: %v", installErr)

	resources.LogLevel("info", "Installed NVIDIA driver")
}

func initialSetupSles(ip string) {
	ensureSlEsRegistration(ip)
	driverVersion := installNvidiaDriverSles(ip)
	installNvidiaComputeUtilsSles(ip, driverVersion)
}

func ensureSlEsRegistration(ip string) {
	// check for zypper locks and clear them if needed.
	clearLocks := "sudo pkill -f zypper 2>/dev/null || true; sudo rm -f /var/run/zypp.pid 2>/dev/null || true; sleep 2"
	_, _ = resources.RunCommandOnNode(clearLocks, ip)

	resources.LogLevel("info", "Checking SLES registration status")
	checkRepos := "sudo zypper lr 2>&1"
	repoStatus, _ := resources.RunCommandOnNode(checkRepos, ip)
	if strings.Contains(repoStatus, "No repositories defined") || strings.Contains(repoStatus, "Warning: No repositories") {
		resources.LogLevel("warn", "No repositories configured, attempting cloud registration")
		registerCmd := "sudo registercloudguest --force-new 2>&1"
		regRes, regErr := resources.RunCommandOnNode(registerCmd, ip)
		resources.LogLevel("debug", "Registration output:\n%s", regRes)

		if regErr != nil || !strings.Contains(regRes, "succeeded") {
			resources.LogLevel("error", "Failed to register with cloud update server: %v", regErr)
			Expect(regErr).ToNot(HaveOccurred(),
				"SLES registration failed, cannot install NVIDIA driver without repos: %v\nOutput: %s", regErr, regRes)
		}
		resources.LogLevel("info", "SLES registration successful")

		// refresh zypper repos after registration to make packages available.
		resources.LogLevel("info", "Refreshing zypper repositories after registration")
		refreshCmd := "sudo zypper --non-interactive ref 2>&1"
		refreshRes, refreshErr := resources.RunCommandOnNode(refreshCmd, ip)
		if refreshErr != nil {
			resources.LogLevel("warn", "Zypper refresh had issues: %v\nOutput: %s", refreshErr, refreshRes)
		}
		resources.LogLevel("debug", "Zypper refresh output:\n%s", refreshRes)
	}
}

func installNvidiaDriverSles(ip string) string {
	// get the current kernel variant to match the correct kmp package.
	getKernel := "uname -r | awk -F'-' '{print $NF}'"
	kernelVariant, kernelErr := resources.RunCommandOnNode(getKernel, ip)
	if kernelErr != nil {
		resources.LogLevel("warn", "Failed to detect kernel variant, defaulting to 'default': %v", kernelErr)
		kernelVariant = "default"
	}
	kernelVariant = strings.TrimSpace(kernelVariant)
	resources.LogLevel("info", "Detected kernel variant: %s", kernelVariant)
	driverPackage := "nvidia-open-driver-G06-signed-cuda-kmp-" + kernelVariant

	// always install latest available - SLES manages driver versions in repos.
	installDriver := "sudo zypper -v --non-interactive in " + driverPackage + " 2>&1"
	res, installDriverErr := resources.RunCommandOnNode(installDriver, ip)
	resources.LogLevel("debug", "Driver installation output:\n%s", res)
	Expect(installDriverErr).ToNot(HaveOccurred(), "error installing driver: %v\nOutput: %s", installDriverErr, res)

	checkInstalled := "rpm -q " + driverPackage
	installedVer, _ := resources.RunCommandOnNode(checkInstalled, ip)
	resources.LogLevel("info", "Installed NVIDIA driver: %s", strings.TrimSpace(installedVer))

	// extract the driver version.
	getDriverVersion := "rpm -q " + driverPackage + " --queryformat '%{VERSION}' | cut -d_ -f1"
	driverVersion, versionErr := resources.RunCommandOnNode(getDriverVersion, ip)
	if versionErr != nil {
		resources.LogLevel("error", "Failed to get driver version: %v", versionErr)
		driverVersion = ""
	}
	driverVersion = strings.TrimSpace(driverVersion)
	resources.LogLevel("info", "Extracted driver version for compute-utils matching: %s", driverVersion)

	resources.LogLevel("info", "Loading NVIDIA kernel module")
	loadModule := "sudo modprobe nvidia && sudo modprobe nvidia-uvm"
	modRes, modErr := resources.RunCommandOnNode(loadModule, ip)
	if modErr != nil {
		resources.LogLevel("warn", "Failed to load NVIDIA module: %v, output: %s", modErr, modRes)
		resources.LogLevel("info", "Checking dmesg for kernel module errors")
		dmesgCheck := "sudo dmesg | grep -i nvidia | tail -20"
		dmesgOut, _ := resources.RunCommandOnNode(dmesgCheck, ip)
		resources.LogLevel("debug", "dmesg output:\n%s", dmesgOut)
		Expect(modErr).ToNot(HaveOccurred(), "error loading NVIDIA kernel module: %v\nOutput: %s", modErr, modRes)
	}

	verifyModule := "lsmod | grep nvidia"
	modCheck, modCheckErr := resources.RunCommandOnNode(verifyModule, ip)
	Expect(modCheckErr).ToNot(HaveOccurred(), "NVIDIA module not loaded: %v", modCheckErr)
	resources.LogLevel("info", "NVIDIA kernel modules loaded:\n%s", strings.TrimSpace(modCheck))

	return driverVersion
}

func installNvidiaComputeUtilsSles(ip, driverVersion string) {
	cudaRepo := "sudo zypper ar https://developer.download.nvidia.com/compute/cuda/repos/sles15/x86_64 cuda"
	_, cudaRepoErr := resources.RunCommandOnNode(cudaRepo, ip)
	if cudaRepoErr != nil && !strings.Contains(cudaRepoErr.Error(), "exists") {
		Expect(cudaRepoErr).NotTo(HaveOccurred(), "error adding cuda repo: %v", cudaRepoErr)
	} else if cudaRepoErr != nil {
		resources.LogLevel("warn", "CUDA repo already exists, proceeding...")
	}
	resources.LogLevel("info", "Added CUDA repository")

	gpgKeys := "sudo zypper --gpg-auto-import-keys ref"
	_, gpgKeysErr := resources.RunCommandOnNode(gpgKeys, ip)
	Expect(gpgKeysErr).ToNot(HaveOccurred(), "error importing gpg keys: %v", gpgKeysErr)

	cmdref := "sudo zypper --non-interactive ref"
	res, cmdErr := resources.RunCommandOnNode(cmdref, ip)
	Expect(cmdErr).ToNot(HaveOccurred(), "error refreshing repos: %v", cmdErr)
	Expect(res).To(ContainSubstring("All repositories have been refreshed."))

	keyImport := "sudo rpm --import " +
		"https://developer.download.nvidia.com/compute/cuda/repos/sles15/x86_64/repodata/repomd.xml.key"
	_, keyImportErr := resources.RunCommandOnNode(keyImport, ip)
	Expect(keyImportErr).ToNot(HaveOccurred(), "error importing key: %v", keyImportErr)
	resources.LogLevel("info", "Imported NVIDIA RPM key")

	checkNvidiaSmi := "which nvidia-smi"
	_, nvidiaSmiErr := resources.RunCommandOnNode(checkNvidiaSmi, ip)
	if nvidiaSmiErr == nil {
		resources.LogLevel("info", "nvidia-smi already available, skipping compute-utils installation")
		return
	}

	// install compute-utils with version pinning to match the installed driver
	var installComputeUtils string
	if driverVersion != "" {
		resources.LogLevel("info", "Installing nvidia-compute-utils-G06 version %s to match driver", driverVersion)
		installComputeUtils = "sudo zypper -v --non-interactive in -r cuda " +
			"'nvidia-compute-utils-G06==" + driverVersion + "' 2>&1"
	} else {
		resources.LogLevel("warn", "Driver version unknown, "+
			"installing latest nvidia-compute-utils-G06 (may cause version mismatch)")
		installComputeUtils = "sudo zypper -v --non-interactive in -r cuda nvidia-compute-utils-G06 2>&1"
	}

	res, installComputeUtilsErr := resources.RunCommandOnNode(installComputeUtils, ip)
	if installComputeUtilsErr != nil {
		resources.LogLevel("error", "Failed to install nvidia-compute-utils-G06 from CUDA repo: %v\nOutput: %s",
			installComputeUtilsErr, res)
		Expect(installComputeUtilsErr).ToNot(HaveOccurred(),
			"error installing nvidia-compute-utils-G06: %v\nOutput: %s", installComputeUtilsErr, res)
	}

	_, finalCheck := resources.RunCommandOnNode(checkNvidiaSmi, ip)
	Expect(finalCheck).ToNot(HaveOccurred(), "nvidia-smi not found after compute-utils installation")
	resources.LogLevel("info", "Successfully installed NVIDIA compute utils")
}

func initialSetupRHEL(ip, nvidiaVersion string) {
	Expect(nvidiaVersion).NotTo(BeEmpty(), "nvidiaVersion parameter is required for RHEL. "+
		"Please set NVIDIA_VERSION environment variable or pass it as a flag to the test.")

	// create a empty dummy repo file to GPU operator acknowledge.
	repoFile := "sudo mkdir -p /etc/yum.repos.d && sudo touch /etc/yum.repos.d/redhat.repo && " +
		"sudo chmod 644 /etc/yum.repos.d/redhat.repo"
	_, repoErr := resources.RunCommandOnNode(repoFile, ip)
	Expect(repoErr).ToNot(HaveOccurred(), "error creating repo file: %v", repoErr)

	resources.LogLevel("info", "Downloading NVIDIA driver version %s from NVIDIA website", nvidiaVersion)
	downloadDriver := "sudo curl -fSsl -O  https://us.download.nvidia.com/tesla/" +
		nvidiaVersion + "/NVIDIA-Linux-x86_64-" + nvidiaVersion + ".run"
	_, downloadErr := resources.RunCommandOnNode(downloadDriver, ip)
	Expect(downloadErr).ToNot(HaveOccurred(), "error downloading NVIDIA driver: %v", downloadErr)
	resources.LogLevel("info", "Downloaded NVIDIA driver version %s", nvidiaVersion)

	checkKernel := "uname -r"
	kernelVersion, kernelCheckErr := resources.RunCommandOnNode(checkKernel, ip)
	Expect(kernelCheckErr).ToNot(HaveOccurred(), "error checking kernel version: %v", kernelCheckErr)
	kernelVersion = strings.TrimSpace(kernelVersion)
	resources.LogLevel("info", "Kernel version: %s", kernelVersion)

	kernelPackages := fmt.Sprintf("sudo yum -y install kernel-devel-%s kernel-headers-%s gcc make acpid pkg-config ",
		kernelVersion, kernelVersion)
	_, kernelErr := resources.RunCommandOnNode(kernelPackages, ip)
	Expect(kernelErr).ToNot(HaveOccurred(), "error installing kernel packages: %v", kernelErr)
	resources.LogLevel("info", "Installed kernel development packages")

	// update sim link so when driver is installed,
	// it will use the correct kernel version and it will find the path.
	kernelPath := "/usr/src/kernels/" + kernelVersion
	kernelSimLinkPath := "/usr/lib/modules/" + kernelVersion + "/build"
	sl := "sudo ln -sf " + kernelPath + " " + kernelSimLinkPath
	_, slErr := resources.RunCommandOnNode(sl, ip)
	Expect(slErr).ToNot(HaveOccurred(), "error creating symlink: %v", slErr)

	driverInstall := "sudo bash NVIDIA-Linux-x86_64-" + nvidiaVersion + ".run --accept-license --silent --no-questions" +
		" --ui=none --kernel-source-path=" + kernelSimLinkPath
	_, installErr := resources.RunCommandOnNode(driverInstall, ip)
	Expect(installErr).ToNot(HaveOccurred(), "error installing NVIDIA driver: %v", installErr)
	resources.LogLevel("info", "Installed NVIDIA driver")

	s := "sudo setenforce 0"
	_, cmdErr := resources.RunCommandOnNode(s, ip)
	Expect(cmdErr).ToNot(HaveOccurred(), "error setting SELinux to permissive mode: %v", cmdErr)
}

func validateNvidiaVersion(ip string) {
	versionCmd := "sudo cat /proc/driver/nvidia/version"

	// TODO: restore CmdNodeRetryCfg
	// cfg := resources.CmdNodeRetryCfg()
	cfg := resources.RetryCfg{
		Attempts:                20,
		Delay:                   10 * time.Second,
		RetryableErrorSubString: []string{"No such file or directory"},
	}

	res, err := resources.RunCommandOnNodeWithRetry(versionCmd, ip, &cfg)
	Expect(err).NotTo(HaveOccurred(), "failed to read driver version: %v", err)
	Expect(res).To(ContainSubstring("NVRM version: NVIDIA"), "NVRM version string not found")
	Expect(res).To(ContainSubstring("GCC version:"), "GCC version string not found")
	Expect(res).To(ContainSubstring("Release Build"), "Release Build string not found")
	Expect(res).To(ContainSubstring("NVIDIA UNIX Open Kernel Module"),
		"NVIDIA UNIX Open Kernel Module string not found")

	resources.LogLevel("info", "NVIDIA driver version:\n%s", res)
}

func validateNvidiaLibMl(ip string) {
	// search for libnvidia-ml library (may have version suffix like .so.1 or .so.580.95.05)
	findCmd := "sudo find /usr/ -name 'libnvidia-ml.so*' 2>/dev/null | head -5"

	res, err := resources.RunCommandOnNode(findCmd, ip)
	Expect(err).NotTo(HaveOccurred(), "failed to find libnvidia-ml.so: %v", err)
	Expect(res).NotTo(BeEmpty(), "libnvidia-ml.so library not found in /usr/")
	Expect(res).To(Or(
		ContainSubstring("/usr/lib64/libnvidia-ml.so"),
		ContainSubstring("/usr/lib/x86_64-linux-gnu/libnvidia-ml.so")),
		"libnvidia-ml.so not found in expected library paths")

	resources.LogLevel("info", "libnvidia-ml.so library found:\n%s", res)
}

func validateNvidiaOperatorDeploy(nodeName string) {
	cmd := fmt.Sprintf("kubectl get node %s  --kubeconfig=%s -o jsonpath=\"{.metadata.labels}\" ",
		strings.TrimSpace(nodeName), resources.KubeConfigFile)

	labelsToFind := []string{
		"\"nvidia.com/gpu.deploy.driver\":" + "\"pre-installed\"",
		"nvidia.com/cuda.driver.major",
		"nvidia.com/gpu.machine",
		"nvidia.com/gpu.count",
		"nvidia.com/gpu.product",
	}
	resources.LogLevel("debug", "Searching for labels: %s", strings.Join(labelsToFind, ", "))

	retryErr := retry.Do(
		func() error {
			res, err := resources.RunCommandHost(cmd)
			resources.LogLevel("debug", "Node labels output: %s", res)
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
		retry.Attempts(40),
		retry.Delay(10*time.Second),
		retry.OnRetry(func(n uint, err error) {
			resources.LogLevel("warn", "Attempt %d failed, retrying to get node labels: %v", n+1, err)
		}))

	Expect(retryErr).NotTo(HaveOccurred(), "failed to get node labels after multiple attempts: %v", retryErr)
}

func validateNvidiaGPU(nodeName string) {
	cmd := fmt.Sprintf("kubectl get node %s -o jsonpath=\"{.status.allocatable}\"", nodeName)
	res, err := resources.RunCommandHost(cmd + " --kubeconfig=" + resources.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred())

	gpuRegex := regexp.MustCompile(`"nvidia\.com/gpu":"(\d+)"`)
	ok := gpuRegex.FindStringSubmatch(res)
	Expect(ok).To(HaveLen(2), "Failed to extract GPU count")

	value := ok[1]
	count, err := strconv.Atoi(value)
	Expect(err).NotTo(HaveOccurred(), "failed to convert GPU value to integer")
	Expect(count).To(BeNumerically(">", 0), "GPU count is not greater than 0")

	resources.LogLevel("info", "Nvidia GPU count found on node %s: %d", nodeName, count)
}

func validateNvidiaToolKit(ip string) {
	toolkit := "sudo ls -l /usr/local/nvidia/toolkit | cat"
	res, err := resources.RunCommandOnNode(toolkit, ip)
	Expect(err).NotTo(HaveOccurred(), "failed to list toolkit directory: %v", err)
	Expect(res).To(ContainSubstring("nvidia-container-runtime"),
		"nvidia-container-runtime not found in toolkit directory")

	resources.LogLevel("info", "Nvidia toolkit directory:\n%s", res)
}

func validateContainerdConfig(ip string) {
	containerdConfigPath := "/var/lib/rancher/rke2/agent/etc/containerd/config.toml"
	checkCmd := "sudo grep nvidia  " + containerdConfigPath

	res, err := resources.RunCommandOnNode(checkCmd, ip)
	Expect(err).NotTo(HaveOccurred(), "failed to grep containerd config or 'nvidia' runtime not found in %s",
		containerdConfigPath)
	Expect(res).To(ContainSubstring("nvidia"), "containerd config does not contain nvidia runtime entry")

	resources.LogLevel("info", "Containerd config contains nvidia runtime entry:\n%s", res)
}

func validateNvidiaRunBinPath(ip string) {
	runtimeBinPath := "/usr/local/nvidia/toolkit/nvidia-container-runtime"
	checkCmd := "sudo stat " + runtimeBinPath

	res, err := resources.RunCommandOnNode(checkCmd, ip)
	Expect(err).NotTo(HaveOccurred(), "nvidia-container-runtime binary not found at %s", runtimeBinPath)

	resources.LogLevel("info", "nvidia-container-runtime binary found at %s", res)
}

func validateNvidiaModule(ip string) error {
	lsmodCmd := "sudo lsmod | grep nvidia"
	modulesToCheck := []string{
		"nvidia",
		"nvidia_uvm",
		"nvidia_drm",
		"nvidia_modeset",
	}

	// TODO: restore CmdNodeRetryCfg
	// cfg := resources.CmdNodeRetryCfg()
	cfg := resources.RetryCfg{
		Attempts:                20,
		Delay:                   10 * time.Second,
		RetryableErrorSubString: []string{"No such file or directory"},
	}

	out, err := resources.RunCommandOnNodeWithRetry(lsmodCmd, ip, &cfg)
	Expect(err).NotTo(HaveOccurred(),
		"failed to find nvidia module via lsmod after multiple attempts: %v", err)

	for _, module := range modulesToCheck {
		if !strings.Contains(out, module) {
			resources.LogLevel("warn", "NVIDIA module %s not found in lsmod output:\n%s\nRetrying...", module, out)

			output, retryErr := resources.RunCommandOnNode(module, ip)
			if !strings.Contains(output, module) {
				return fmt.Errorf("NVIDIA module %s not found in lsmod output:\n%s\n%v", module, output, retryErr)
			}
		}
	}

	resources.LogLevel("info", "NVIDIA modules found:\n%s", out)

	return nil
}

func validateNvidiaBenchmarkPodStatus() {
	cmd := fmt.Sprintf("kubectl get pod nbody-gpu-benchmark -n test-nvidia-benchmark "+
		"--kubeconfig=%s -o jsonpath='{.status.phase}'",
		resources.KubeConfigFile)

	var podStatus string
	var err error
	retryErr := retry.Do(
		func() error {
			podStatus, err = resources.RunCommandHost(cmd)
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
			resources.LogLevel("warn", "Attempt %d failed, retrying to get benchmark pod status: %v", n+1, err)
		}),
	)
	Expect(retryErr).NotTo(HaveOccurred(), "failed to get benchmark pod status "+
		"after multiple attempts: %v", retryErr)

	resources.LogLevel("info", "Benchmark pod status: %s", podStatus)
}

func validateBenchmark() {
	benchmarkLogs := "kubectl logs nbody-gpu-benchmark -n test-nvidia-benchmark " +
		"--kubeconfig=" + resources.KubeConfigFile
	logs, logErr := resources.RunCommandHost(benchmarkLogs)
	Expect(logErr).NotTo(HaveOccurred(), "Failed to get benchmark pod logs")

	Expect(logs).To(ContainSubstring(" CUDA device: [Tesla T4]"),
		"CUDA device not found in benchmark logs")
	Expect(logs).To(ContainSubstring("billion interactions per second"),
		"Benchmark logs did not contain 'billion interactions per second info'")

	resources.LogLevel("info", "Benchmark logs contain nvidia device and performance info:\n%s", logs)
}
