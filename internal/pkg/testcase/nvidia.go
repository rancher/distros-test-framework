package testcase

import (
	"encoding/json"
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

type nvidiaOperatorConfig struct {
	workload              string
	operatorManagedDriver bool
	nriEnabled            bool
}

// isDebianFamily reports whether the OS is Ubuntu/Debian-based.
func isDebianFamily(os string) bool {
	return os == "ubuntu" || strings.HasPrefix(os, "debian")
}

// isRHELFamily reports whether the OS is a RHEL derivative.
func isRHELFamily(os string) bool {
	return strings.HasPrefix(os, "rhel") || strings.HasPrefix(os, "centos") ||
		strings.HasPrefix(os, "rocky") || strings.HasPrefix(os, "oracle")
}

// isSUSEFamily reports whether the OS is a SUSE/openSUSE variant.
func isSUSEFamily(os string) bool {
	return strings.HasPrefix(os, "sles") || strings.HasPrefix(os, "suse") ||
		strings.HasPrefix(os, "opensuse")
}

// runNvidiaDriverSetup installs the NVIDIA driver for the node's OS family.
// Returns false when the OS is unsupported so the caller can stop.
func runNvidiaDriverSetup(targetNodeIP, nodeOS, nvidiaVersion string) bool {
	osLower := strings.ToLower(nodeOS)
	switch {
	case isDebianFamily(osLower):
		resources.LogLevel("info", "Proceeding with Ubuntu setup for NVIDIA driver installation version: %s", nvidiaVersion)
		initialSetupUbuntu(targetNodeIP, nvidiaVersion)
	case isRHELFamily(osLower):
		resources.LogLevel("info", "Proceeding with RHEL setup for NVIDIA driver installation version: %s", nvidiaVersion)
		initialSetupRHEL(targetNodeIP, nvidiaVersion)
	case isSUSEFamily(osLower):
		resources.LogLevel("info", "Proceeding with SLES setup for NVIDIA driver installation with latest available driver")
		initialSetupSles(targetNodeIP)
	default:
		resources.LogLevel("error", "Unsupported OS: %s version: %s", nodeOS, nvidiaVersion)
		return false
	}

	return true
}

func TestNvidiaGPUFunctionality(cluster *driver.Cluster, nvidiaVersion string) {
	// for now we are only testing integration with the first server in the cluster.
	targetNodeIP := cluster.ServerIPs[0]
	nodeOs = cluster.NodeOS
	operatorConfig := nvidiaOperatorWorkload(nodeOs)

	// SLE Micro is read-only / transactional; NVIDIA driver/operator path isn't supported here.
	// Bail before hardware detection so we don't install pciutils on a host we can't drive anyway.
	if strings.EqualFold(nodeOs, "slemicro") {
		resources.LogLevel("warn", "Skipping NVIDIA test on %q: SLE Micro is unsupported", nodeOs)
		return
	}

	verifyGPUHardwarePresence(targetNodeIP, nodeOs)

	if operatorConfig.operatorManagedDriver {
		resources.LogLevel("info", "Using the SUSE precompiled driver managed by the GPU Operator")
	} else {
		if !runNvidiaDriverSetup(targetNodeIP, nodeOs, nvidiaVersion) {
			return
		}

		validateNvidiaVersion(targetNodeIP)
		validateNvidiaLibMl(targetNodeIP, false)
	}

	workloadErr := resources.ManageWorkload("apply", operatorConfig.workload)
	Expect(workloadErr).NotTo(HaveOccurred(), "nvidia operator manifests not deployed")

	resources.LogLevel("info", "Waiting needed as per documentation for operator to restart containerd and stabilize")
	time.Sleep(60 * time.Second)

	nodeName, err := resources.RunCommandHost("kubectl get nodes -o jsonpath='{.items[0].metadata.name}' " +
		"--kubeconfig=" + resources.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred(), "failed to get node name: %v", err)
	Expect(nodeName).NotTo(BeEmpty(), "Node name is empty")

	validateNvidiaOperatorDeploy(nodeName, operatorConfig.operatorManagedDriver)
	if operatorConfig.operatorManagedDriver {
		validateNvidiaVersion(targetNodeIP)
		validateNvidiaLibMl(targetNodeIP, true)
	}

	validateNvidiaGPU(nodeName)
	validateNvidiaRuntime(targetNodeIP, operatorConfig)

	workloadErr = resources.ManageWorkload("apply", "nvidia-benchmark.yaml")
	Expect(workloadErr).NotTo(HaveOccurred(), "nvidia benchmark manifests not deployed")
	validateNvidiaBenchmarkPodStatus()
	validateBenchmark()
}

func validateNvidiaRuntime(ip string, config nvidiaOperatorConfig) {
	validateNvidiaRunBinPath(ip)
	if !config.nriEnabled {
		validateContainerdConfig(ip)
	}
	validateNvidiaToolKit(ip)

	err := validateNvidiaModule(ip)
	Expect(err).NotTo(HaveOccurred(), "NVIDIA module not found: %v", err)
}

func nvidiaOperatorWorkload(nodeOS string) nvidiaOperatorConfig {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(nodeOS)), "sles") {
		return nvidiaOperatorConfig{
			workload:              "nvidia-operator-sles.yaml",
			operatorManagedDriver: true,
		}
	}

	return nvidiaOperatorConfig{
		workload:   "nvidia-operator.yaml",
		nriEnabled: true,
	}
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
	osLower := strings.ToLower(nodeOs)
	switch {
	case isDebianFamily(osLower):
		installCmd = "sudo apt update && sudo DEBIAN_FRONTEND=noninteractive apt install -y pciutils"
	case isRHELFamily(osLower):
		installCmd = "sudo yum install -y pciutils || sudo dnf install -y pciutils"
	case isSUSEFamily(osLower):
		installCmd = "sudo zypper --non-interactive install pciutils"
	default:
		resources.LogLevel("warn", "Unknown OS %q, attempting all known package managers", nodeOs)
		installCmd = "sudo zypper --non-interactive install pciutils || " +
			"sudo yum install -y pciutils || sudo dnf install -y pciutils || sudo apt install -y pciutils"
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

	ensureSlesModuleForRunningKernel(ip)

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

// ensureSlesModuleForRunningKernel reboots into the updated kernel when the
// SLES repo ships a KMP built for a newer kernel than the AMI is running.
func ensureSlesModuleForRunningKernel(ip string) {
	if _, modinfoErr := resources.RunCommandOnNode("sudo modinfo nvidia > /dev/null 2>&1", ip); modinfoErr == nil {
		return
	}

	runningKernel, _ := resources.RunCommandOnNode("uname -r", ip)
	resources.LogLevel("info", "nvidia module not available for running kernel %s, "+
		"rebooting into the updated kernel", strings.TrimSpace(runningKernel))

	rebootNvidiaNodeAndWait(ip)

	newKernel, _ := resources.RunCommandOnNode("uname -r", ip)
	resources.LogLevel("info", "Reboot complete, running kernel is now: %s", strings.TrimSpace(newKernel))
}

func rebootNvidiaNodeAndWait(ip string) {
	// The SSH connection can drop mid-command, so an error return is expected.
	_, _ = resources.RunCommandOnNode("sudo systemctl reboot", ip)
	time.Sleep(30 * time.Second)

	sshErr := resources.WaitForSSHReadyWithTimeout(ip, 5*time.Minute)
	Expect(sshErr).ToNot(HaveOccurred(), "node did not come back from reboot: %v", sshErr)

	retryErr := retry.Do(
		func() error {
			out, err := resources.RunCommandOnNode(
				"sudo systemctl is-active rke2-server 2>/dev/null || sudo systemctl is-active k3s 2>/dev/null", ip)
			if err != nil || strings.TrimSpace(out) != "active" {
				return fmt.Errorf("kubernetes service not active yet: %s", strings.TrimSpace(out))
			}

			return nil
		},
		retry.Attempts(30),
		retry.Delay(10*time.Second),
		retry.DelayType(retry.FixedDelay),
	)
	Expect(retryErr).ToNot(HaveOccurred(), "kubernetes service did not come back after reboot: %v", retryErr)
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

	disableNouveauIfLoaded(ip)

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

	// Dummy redhat.repo for the driver daemonset's hostPath mount; must run
	// AFTER the last yum call — RHEL 10's subscription-manager deletes it.
	repoFile := "sudo mkdir -p /etc/yum.repos.d && sudo touch /etc/yum.repos.d/redhat.repo && " +
		"sudo chmod 644 /etc/yum.repos.d/redhat.repo"
	_, repoErr := resources.RunCommandOnNode(repoFile, ip)
	Expect(repoErr).ToNot(HaveOccurred(), "error creating repo file: %v", repoErr)

	s := "sudo setenforce 0"
	_, cmdErr := resources.RunCommandOnNode(s, ip)
	Expect(cmdErr).ToNot(HaveOccurred(), "error setting SELinux to permissive mode: %v", cmdErr)
}

func disableNouveauIfLoaded(ip string) {
	modules, err := resources.RunCommandOnNode("sudo lsmod", ip)
	Expect(err).ToNot(HaveOccurred(), "failed to inspect loaded kernel modules: %v", err)
	if !kernelModuleLoaded(modules, "nouveau") {
		return
	}

	resources.LogLevel("info", "Disabling the Nouveau driver before installing NVIDIA")
	blacklistCmd := "printf 'blacklist nouveau\\noptions nouveau modeset=0\\n' | " +
		"sudo tee /etc/modprobe.d/blacklist-nouveau.conf >/dev/null && sudo dracut --force --regenerate-all"
	_, blacklistErr := resources.RunCommandOnNode(blacklistCmd, ip)
	Expect(blacklistErr).ToNot(HaveOccurred(), "failed to blacklist Nouveau and rebuild initramfs: %v", blacklistErr)

	rebootNvidiaNodeAndWait(ip)
	modules, err = resources.RunCommandOnNode("sudo lsmod", ip)
	Expect(err).ToNot(HaveOccurred(), "failed to inspect modules after disabling Nouveau: %v", err)
	Expect(kernelModuleLoaded(modules, "nouveau")).To(BeFalse(), "Nouveau is still loaded after reboot")
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

func validateNvidiaLibMl(ip string, operatorManagedDriver bool) {
	// search for libnvidia-ml library (may have version suffix like .so.1 or .so.580.95.05)
	libraryRoot := nvidiaDriverLibraryRoot(operatorManagedDriver)
	findCmd := "sudo find " + libraryRoot + " -name 'libnvidia-ml.so*' 2>/dev/null | head -5"

	res, err := resources.RunCommandOnNode(findCmd, ip)
	Expect(err).NotTo(HaveOccurred(), "failed to find libnvidia-ml.so: %v", err)
	Expect(res).NotTo(BeEmpty(), "libnvidia-ml.so library not found in %s", libraryRoot)
	if operatorManagedDriver {
		Expect(res).To(ContainSubstring("/run/nvidia/driver/usr/lib64/libnvidia-ml.so"),
			"libnvidia-ml.so not found in the managed SLES driver path")
	} else {
		Expect(res).To(Or(
			ContainSubstring("/usr/lib64/libnvidia-ml.so"),
			ContainSubstring("/usr/lib/x86_64-linux-gnu/libnvidia-ml.so")),
			"libnvidia-ml.so not found in expected host library paths")
	}

	resources.LogLevel("info", "libnvidia-ml.so library found:\n%s", res)
}

func validateNvidiaOperatorDeploy(nodeName string, operatorManagedDriver bool) {
	retryErr := retry.Do(
		func() error {
			res, err := resources.RunHostArgs("kubectl", "get", "node", strings.TrimSpace(nodeName),
				"--kubeconfig="+resources.KubeConfigFile, "-o", "json")
			if err != nil {
				return fmt.Errorf("failed to get node labels: %w", err)
			}

			if err := validateNvidiaNodeLabels(res, operatorManagedDriver); err != nil {
				return err
			}

			if !operatorManagedDriver {
				return nil
			}

			return validateManagedNvidiaDriverReady()
		},
		retry.Attempts(40),
		retry.Delay(10*time.Second),
		retry.DelayType(retry.FixedDelay),
		retry.OnRetry(func(n uint, err error) {
			resources.LogLevel("warn", "Attempt %d failed, retrying to get node labels: %v", n+1, err)
		}))

	Expect(retryErr).NotTo(HaveOccurred(), "NVIDIA Operator did not converge after multiple attempts: %v", retryErr)
}

func nvidiaDriverLibraryRoot(operatorManagedDriver bool) string {
	if operatorManagedDriver {
		return "/run/nvidia/driver"
	}

	return "/usr"
}

func nvidiaDriverLabelValue(operatorManagedDriver bool) string {
	if operatorManagedDriver {
		return "true"
	}

	return "pre-installed"
}

func validateNvidiaNodeLabels(output string, operatorManagedDriver bool) error {
	var node struct {
		Metadata struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(output), &node); err != nil {
		return fmt.Errorf("failed to decode node labels: %w", err)
	}

	driverLabel := nvidiaDriverLabelValue(operatorManagedDriver)
	if node.Metadata.Labels["nvidia.com/gpu.deploy.driver"] != driverLabel {
		return fmt.Errorf("label nvidia.com/gpu.deploy.driver=%s not found", driverLabel)
	}

	for _, label := range []string{
		"nvidia.com/cuda.driver.major",
		"nvidia.com/gpu.machine",
		"nvidia.com/gpu.count",
		"nvidia.com/gpu.product",
	} {
		if _, found := node.Metadata.Labels[label]; !found {
			return fmt.Errorf("label %s not found", label)
		}
	}

	return nil
}

func validateManagedNvidiaDriverReady() error {
	policyState, err := resources.RunHostArgs("kubectl", "get", "clusterpolicy", "cluster-policy",
		"--kubeconfig="+resources.KubeConfigFile, "-o", "jsonpath={.status.state}")
	if err != nil {
		return fmt.Errorf("failed to get NVIDIA ClusterPolicy state: %w", err)
	}

	daemonSetStatus, err := resources.RunHostArgs("kubectl", "get", "daemonset", "-n", "gpu-operator",
		"-l", "app=nvidia-driver-daemonset", "--kubeconfig="+resources.KubeConfigFile,
		"-o", "jsonpath={.items[0].status.desiredNumberScheduled}:{.items[0].status.numberReady}")
	if err != nil {
		return fmt.Errorf("failed to get NVIDIA driver DaemonSet state: %w", err)
	}

	return managedNvidiaDriverReady(policyState, daemonSetStatus)
}

func managedNvidiaDriverReady(policyState, daemonSetStatus string) error {
	if !strings.EqualFold(strings.TrimSpace(policyState), "ready") {
		return fmt.Errorf("NVIDIA ClusterPolicy is not ready: %q", strings.TrimSpace(policyState))
	}

	parts := strings.Split(strings.TrimSpace(daemonSetStatus), ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid NVIDIA driver DaemonSet state %q", daemonSetStatus)
	}
	desired, desiredErr := strconv.Atoi(parts[0])
	ready, readyErr := strconv.Atoi(parts[1])
	if desiredErr != nil || readyErr != nil || desired == 0 || ready != desired {
		return fmt.Errorf("NVIDIA driver DaemonSet is not ready: %q", daemonSetStatus)
	}

	return nil
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
	var out string
	retryErr := retry.Do(
		func() error {
			var err error
			out, err = resources.RunCommandOnNode("sudo lsmod", ip)
			if err != nil {
				return fmt.Errorf("failed to list kernel modules: %w", err)
			}

			missing := missingKernelModules(out, []string{"nvidia", "nvidia_uvm"})
			if len(missing) != 0 {
				return fmt.Errorf("required NVIDIA modules not loaded: %s", strings.Join(missing, ", "))
			}

			return nil
		},
		retry.Attempts(20),
		retry.Delay(10*time.Second),
		retry.DelayType(retry.FixedDelay),
	)
	if retryErr != nil {
		return fmt.Errorf("NVIDIA compute modules not ready: %w\nlsmod output:\n%s", retryErr, out)
	}

	resources.LogLevel("info", "NVIDIA modules found:\n%s", out)

	return nil
}

func missingKernelModules(output string, required []string) []string {
	loaded := make(map[string]struct{})
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 0 {
			loaded[fields[0]] = struct{}{}
		}
	}

	missing := make([]string, 0, len(required))
	for _, module := range required {
		if _, found := loaded[module]; !found {
			missing = append(missing, module)
		}
	}

	return missing
}

func kernelModuleLoaded(output, module string) bool {
	return len(missingKernelModules(output, []string{module})) == 0
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

			return benchmarkPodPhaseError(podStatus)
		},
		retry.Attempts(20),
		retry.Delay(5*time.Second),
		retry.DelayType(retry.FixedDelay),
		retry.OnRetry(func(n uint, err error) {
			resources.LogLevel("warn", "Attempt %d failed, retrying to get benchmark pod status: %v", n+1, err)
		}),
	)
	Expect(retryErr).NotTo(HaveOccurred(), "failed waiting for benchmark pod to succeed: %v", retryErr)

	resources.LogLevel("info", "Benchmark pod status: %s", podStatus)
}

func benchmarkPodPhaseError(podStatus string) error {
	podStatus = strings.TrimSpace(podStatus)
	switch podStatus {
	case "Succeeded":
		return nil
	case "Failed":
		return retry.Unrecoverable(errors.New("benchmark pod failed"))
	default:
		return fmt.Errorf("benchmark pod phase is %q, waiting for Succeeded", podStatus)
	}
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
