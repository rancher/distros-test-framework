package testcase

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

const (
	errDirNotFound   = " no such file or directory"
	errNoMount       = " no mount point specified"
	errMount         = " mount point does not exist"
	errMountNotFound = " not found"
	errNotMounted    = " not mounted"
)

func TestKillAllUninstall(cluster *driver.Cluster, cfg *config.Env) {
	productDataDir := "/var/lib/rancher/" + cluster.Config.Product

	exportToIPs := []string{cluster.ServerIPs[0]}
	if len(cluster.AgentIPs) > 0 {
		exportToIPs = append(exportToIPs, cluster.AgentIPs[0])
	}
	// exporting binary directories to only one server node and one agent if available,
	// so when script test runs, it can already have the paths needed.
	for _, ip := range exportToIPs {
		Expect(ip).NotTo(BeEmpty(), "IP address cannot be empty")

		exportDirErr := exportBinDirs(ip, "crictl", "kubectl", "ctr")
		Expect(exportDirErr).NotTo(HaveOccurred(), "failed to export binary directories: %v on ip: %s", exportDirErr, ip)
	}

	scpTestScripts(cluster)
	mountBindDataDir(cluster, productDataDir)

	killAllValidations(cluster, "true", true)

	resources.LogLevel("info", "umounting data dir: %s since kill all after v1.30.4 doesn't unmount if previously mounted",
		productDataDir)
	umountDataDir(cluster, productDataDir)

	resources.LogLevel("info", "running uninstall script and tests after kill all already ran")
	uninstallValidations(cluster, true)

	resources.LogLevel("info", "reinstalling product on first server only after uninstall")
	reInstallServerProduct(cluster, cfg)

	resources.LogLevel("info", "running uninstall tests without mounting any data dir after reinstall")
	// agent is false here meaning that we are not testing on agent anymore, because it was already tested and to avoid
	// time and resource consuming, we just reinstall and test again on server only.
	uninstallValidations(cluster, false)

	resources.LogLevel("info", "reinstalling again to test uninstall script alone with file removal safety")
	reInstallServerProduct(cluster, cfg)

	// for now only rke2 will support this file safety removal test.
	// since now we needed to mount a fake remote fs, we will handle uninstall tests based on that, because
	// the uninstall script will try to remove the kubelet dir and return device or resource busy error.
	// ( this is expected and handled in the test script )
	if cluster.Config.Product == "rke2" {
		createFakeRemoteFs(cluster)
	}

	uninstallValidations(cluster, false)
}

// exportBinDirs finds and exports binary paths to the specified node.
func exportBinDirs(ip string, binaries ...string) error {
	if ip == "" {
		return errors.New("need at least one IP address")
	}

	binPaths, err := resources.FindBinaries(ip, binaries...)
	if err != nil {
		return fmt.Errorf("failed to find binaries: %w", err)
	}

	envVars := make(map[string]string)
	for bin, dir := range binPaths {
		varName := strings.ToUpper(bin) + "_BIN_DIR"
		envVars[varName] = dir
	}

	exportErr := resources.ExportEnvProfileNode([]string{ip}, envVars, "bin_paths.sh")
	if exportErr != nil {
		return fmt.Errorf("failed to export environment variables: %w", exportErr)
	}

	return nil
}

func scpTestScripts(cluster *driver.Cluster) {
	killAllLocalPath := resources.BasePath() + "/scripts/kill-all_test.sh"
	killAllRemotePath := "/var/tmp/kill-all_test.sh"
	scpErr := resources.RunScp(cluster, cluster.ServerIPs[0], []string{killAllLocalPath}, []string{killAllRemotePath})
	Expect(scpErr).NotTo(HaveOccurred(), "failed to scp kill all test script to server")

	if len(cluster.AgentIPs) > 0 {
		scpAgentErr := resources.RunScp(
			cluster, cluster.AgentIPs[0],
			[]string{killAllLocalPath},
			[]string{killAllRemotePath})
		Expect(scpAgentErr).NotTo(HaveOccurred(), "failed to scp kill all test script to agent")
	}

	uninstallLocalPath := resources.BasePath() + "/scripts/uninstall_test.sh"
	uninstallRemotePath := "/var/tmp/uninstall_test.sh"
	scpUninstallErr := resources.RunScp(
		cluster,
		cluster.ServerIPs[0],
		[]string{uninstallLocalPath},
		[]string{uninstallRemotePath},
	)
	Expect(scpUninstallErr).NotTo(HaveOccurred(), "failed to scp uninstall test script to server")

	if len(cluster.AgentIPs) > 0 {
		scpUninstallAgentErr := resources.RunScp(
			cluster,
			cluster.AgentIPs[0],
			[]string{uninstallLocalPath},
			[]string{uninstallRemotePath},
		)
		Expect(scpUninstallAgentErr).NotTo(HaveOccurred(), "failed to scp uninstall test script to agent")
	}
}

func mountBindDataDir(cluster *driver.Cluster, productDataDir string) {
	err := resources.MountBind([]string{cluster.ServerIPs[0]}, productDataDir+"/server", productDataDir+"/server")
	if err != nil {
		if strings.Contains(err.Error(), " mount point does not exist") {
			resources.LogLevel("info", "data dir not mounted on server, mount point does not exist: %v", err)
		} else {
			Expect(err).NotTo(HaveOccurred(), "failed to mount bind server data dir for server on server nodes")
		}
	}

	if len(cluster.AgentIPs) > 0 {
		err = resources.MountBind([]string{cluster.AgentIPs[0]}, productDataDir+"/agent", productDataDir+"/agent")
		if err != nil {
			if strings.Contains(err.Error(), " mount point does not exist") {
				resources.LogLevel("info", "data dir not mounted on agent, mount point does not exist: %v", err)
			} else {
				Expect(err).NotTo(HaveOccurred(), "failed to mount bind agent data dir for agent on agent nodes")
			}
		}
	}
}

func umountDataDir(cluster *driver.Cluster, productDataDir string) {
	err := umountAllProductDir(productDataDir, cluster.ServerIPs[0], "server")
	Expect(err).NotTo(HaveOccurred(), "failed to umount %s on server node %s",
		productDataDir, cluster.ServerIPs[0])

	if len(cluster.AgentIPs) > 0 {
		umountAgentErr := umountAllProductDir(productDataDir, cluster.AgentIPs[0], "agent")
		Expect(umountAgentErr).NotTo(HaveOccurred(), "failed to umount %s on agent node %s",
			productDataDir, cluster.AgentIPs[0])
	}
}

func umountAllProductDir(productDataDir, nodeIP, nodeType string) error {
	forceUmountCmd := "sudo umount -f -R " + productDataDir + "/server/"

	if nodeType == "agent" {
		forceUmountCmd = "sudo umount -f -R " + productDataDir + "/agent/"
	}
	umountRes, umountErr := resources.RunCommandOnNode(forceUmountCmd, nodeIP)
	if umountErr != nil {
		if !isExpectedMountError(umountErr, productDataDir, nodeIP) {
			return fmt.Errorf("umount failed on node ATTEMPT 1 %s: %v", nodeIP, umountErr)
		}
		resources.LogLevel("debug", "not mounted %s on node %s, mount point does not exist: %v",
			productDataDir, nodeIP, umountErr)
	}
	Expect(umountRes).To(BeEmpty(), "failed to umount on node %s", nodeIP)

	activeMountsErr := umountActiveMounts(productDataDir, nodeIP)
	Expect(activeMountsErr).NotTo(HaveOccurred(), "failed to find and umount active mount points")

	findMountPatternsErr := findMountByPatterns(productDataDir, nodeIP)
	Expect(findMountPatternsErr).NotTo(HaveOccurred(), "failed to find mount and umount by patterns on node %s", nodeIP)

	resources.LogLevel("info", "umount completed for %s on node %s type %s", productDataDir, nodeIP, nodeType)

	return nil
}

func findMountByPatterns(path, nodeIP string) error {
	patterns := "find " + path + " \\( " +
		"-path '*/containerd/tmpmounts*' -o " +
		"-path '*/crio/tmpmounts*' -o " +
		"-path '*/docker/tmpmounts*' -o " +
		"-path '*/kubelet/tmpmounts*' -o " +
		"-name 'tmpmount*' -o " +
		"-name '*-mount*' " +
		"\\) -type d 2>/dev/null || true"

	mountPath, findMountErr := resources.RunCommandOnNode(patterns, nodeIP)
	if findMountErr != nil {
		return fmt.Errorf("failed to find mount paths on node %s: %v", nodeIP, findMountErr)
	}

	if mountPath == "" {
		resources.LogLevel("info", "no mount paths found on node %s", nodeIP)
		return nil
	}

	resources.LogLevel("info", "found mount paths on node %s:\n%s", nodeIP, mountPath)
	mountPaths := strings.Split(strings.TrimSpace(mountPath), "\n")
	for _, p := range mountPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		checkMountCmd := "mount | grep -E '^[^ ]+ " + p + " ' || grep -E '^[^ ]+ " + p + " ' /proc/mounts || true"
		mountCheck, checkMountErr := resources.RunCommandOnNode(checkMountCmd, nodeIP)
		if checkMountErr != nil {
			return fmt.Errorf("failed to find for mount pattern on node %s: %v", nodeIP, checkMountErr)
		}

		if mountCheck == "" {
			resources.LogLevel("info", "no active mount found on node %s for path %s", nodeIP, p)
			continue
		}

		resources.LogLevel("info", "found active mount on node %s: %s\nmount: %s", nodeIP, p, mountCheck)

		umountCmd := "sudo umount -f -R " + p + " 2>/dev/null || sudo umount -l " + p + " 2>/dev/null || true"
		res, umountErr := resources.RunCommandOnNode(umountCmd, nodeIP)
		if !isExpectedMountError(umountErr, p, nodeIP) {
			return fmt.Errorf("failed to umount %s on node %s: %v", p, nodeIP, umountErr)
		}
		resources.LogLevel("debug", "not mounted %s on node %s, mount point does not exist: %v",
			p, nodeIP, umountErr)

		Expect(res).To(BeEmpty(), "failed to umount %s on node %s", p, nodeIP)
	}

	return nil
}

func umountActiveMounts(path, nodeIP string) error {
	findActiveMountsCmd := "mount | grep -E ' " + path + ".*tmpmount| " + path +
		".*-mount' | awk '{print $3}' || grep -E ' " + path + ".*tmpmount| " + path +
		".*-mount' /proc/mounts | awk '{print $2}' || true"

	activeMounts, err := resources.RunCommandOnNode(findActiveMountsCmd, nodeIP)
	if err != nil {
		return fmt.Errorf("failed to find active mount points on node %s: %v", nodeIP, err)
	}

	if activeMounts == "" {
		resources.LogLevel("info", "no active mount points found on node %s for path %s", nodeIP, path)
		return nil
	}

	mountPoints := strings.Split(strings.TrimSpace(activeMounts), "\n")
	for _, mountPoint := range mountPoints {
		mountPoint = strings.TrimSpace(mountPoint)
		if mountPoint == "" {
			resources.LogLevel("info", "skipping, empty mount point on node %s", nodeIP)
			continue
		}

		resources.LogLevel("info", "found additional active mount point on node %s: %s", nodeIP, mountPoint)

		umountCmd := "sudo umount -f " + mountPoint + " 2>/dev/null || sudo umount -l " + mountPoint +
			" 2>/dev/null || true"
		umountRes, umountErr := resources.RunCommandOnNode(umountCmd, nodeIP)
		if !isExpectedMountError(umountErr, path, nodeIP) {
			return fmt.Errorf("failed to umount additional mount point %s on node %s: %v\nresponse: %v",
				mountPoint, nodeIP, umountErr, umountRes)
		}
		resources.LogLevel("debug", "not mounted %s on node %s, mount point does not exist: %v",
			mountPoint, nodeIP, umountErr)
	}

	return nil
}

func isExpectedMountError(err error, path, nodeIP string) bool {
	var errMsg string
	var exitError *ssh.ExitError
	if errors.As(err, &exitError) {
		errMsg = exitError.Msg()
	} else {
		errMsg = err.Error()
	}

	expectedErrors := []string{
		errNotMounted,
		errMount,
		errNoMount,
		errDirNotFound,
		errMountNotFound,
	}

	for _, expectedErr := range expectedErrors {
		if strings.Contains(errMsg, expectedErr) {
			resources.LogLevel("warn", "expected mount error for %s on node %s: %v", path, nodeIP, errMsg)

			return false
		}
	}

	resources.LogLevel("error", "mount error for %s on node %s: %v", path, nodeIP, errMsg)

	return true
}

func killAllValidations(cluster *driver.Cluster, mount string, agent bool) {
	killAllErr := resources.ManageProductCleanup(cluster.Config.Product, "server", cluster.ServerIPs[0], "killall")
	Expect(killAllErr).NotTo(HaveOccurred(), "failed to run killall for product: %v server", cluster.Config.Product)

	killallPattern := fmt.Sprintf("%s.*killall|killall.*%s", cluster.Config.Product, cluster.Config.Product)
	waitErr := resources.CheckProcessCompletion(cluster.ServerIPs[0], killallPattern, 10, 10*time.Second)
	Expect(waitErr).NotTo(HaveOccurred(), "failed waiting for killall process to complete: %v", waitErr)

	killTesCmd := "sudo bash /var/tmp/kill-all_test.sh -mount " + mount
	res, runKillAllErr := resources.RunCommandOnNode(killTesCmd, cluster.ServerIPs[0])
	Expect(runKillAllErr).NotTo(HaveOccurred(), "failed to run kill all test script on server: %v", runKillAllErr)
	Expect(strings.TrimSpace(res)).To(ContainSubstring(
		"All killall operations were successful!"),
		"failed to run kill all test script on server")

	resources.LogLevel("debug", "kill all test script output on server: %s", res)

	if agent && len(cluster.AgentIPs) > 0 {
		killAllAgentErr := resources.ManageProductCleanup(cluster.Config.Product, "agent", cluster.AgentIPs[0], "killall")
		Expect(killAllAgentErr).NotTo(HaveOccurred(), "failed to run killall for product: %v agent", cluster.Config.Product)

		waitAgentErr := resources.CheckProcessCompletion(cluster.AgentIPs[0], killallPattern, 10, 10*time.Second)
		Expect(waitAgentErr).NotTo(HaveOccurred(), "failed waiting for agent killall process to complete: %v", waitAgentErr)

		res, runKillAllErr = resources.RunCommandOnNode(killTesCmd, cluster.AgentIPs[0])
		Expect(runKillAllErr).NotTo(HaveOccurred(),
			"failed to run kill all test script on agent: %v", runKillAllErr)
		Expect(strings.TrimSpace(res)).To(ContainSubstring(
			"All killall operations were successful!"),
			"failed to run kill all test script on agent")
	}

	resources.LogLevel("info", "kill all test script went through successfully with mount: %s", mount)
}

func uninstallValidations(cluster *driver.Cluster, agent bool) {
	uninstalErr := resources.ManageProductCleanup(cluster.Config.Product, "server", cluster.ServerIPs[0], "uninstall")
	Expect(uninstalErr).NotTo(HaveOccurred(), "failed to run uninstall for product: %v server", cluster.Config.Product)

	uninstallPattern := fmt.Sprintf("%s.*uninstall|uninstall.*%s", cluster.Config.Product, cluster.Config.Product)
	waitErr := resources.CheckProcessCompletion(cluster.ServerIPs[0], uninstallPattern, 10, 10*time.Second)
	Expect(waitErr).NotTo(HaveOccurred(), "failed waiting for uninstall process to complete: %v", waitErr)

	uninstallTestCmd := "sudo bash /var/tmp/uninstall_test.sh -p " + cluster.Config.Product
	res, uninstallCmdErr := resources.RunCommandOnNode(uninstallTestCmd, cluster.ServerIPs[0])
	Expect(uninstallCmdErr).NotTo(HaveOccurred(), "failed to run uninstall test script on server")
	Expect(strings.TrimSpace(res)).To(ContainSubstring(
		"All uninstall operations were successful!"),
		"failed to run uninstall test script on server")

	resources.LogLevel("debug", "uninstall test script output on server: %s", res)

	if agent && len(cluster.AgentIPs) > 0 {
		uninstalAgentErr := resources.ManageProductCleanup(cluster.Config.Product, "agent", cluster.AgentIPs[0], "uninstall")
		Expect(uninstalAgentErr).NotTo(HaveOccurred(),
			"failed to run uninstall for product: %v agent",
			cluster.Config.Product)

		waitAgentErr := resources.CheckProcessCompletion(cluster.AgentIPs[0], uninstallPattern, 10, 10*time.Second)
		Expect(waitAgentErr).NotTo(HaveOccurred(), "failed waiting for agent uninstall process to complete: %v",
			waitAgentErr)

		res, uninstallCmdErr = resources.RunCommandOnNode(uninstallTestCmd, cluster.AgentIPs[0])
		Expect(uninstallCmdErr).NotTo(HaveOccurred(), "failed to run uninstall test script on agent")
		Expect(strings.TrimSpace(res)).To(ContainSubstring(
			"All uninstall operations were successful!"),
			"failed to run uninstall test script on agent")
	}

	resources.LogLevel("info", "uninstall test script went through successfully")
}

// reInstallServerProduct reinstalls the product on the first server node only.
func reInstallServerProduct(cluster *driver.Cluster, cfg *config.Env) {
	installErr := resources.InstallProduct(cluster, cluster.ServerIPs[0], cfg.InstallVersion)
	Expect(installErr).NotTo(HaveOccurred(), "failed to install product: %v", installErr)

	enableErr := resources.EnableAndStartService(cluster, cluster.ServerIPs[0], "server")
	Expect(enableErr).NotTo(HaveOccurred(), "failed to enable and start service: %v", enableErr)
}

func createFakeRemoteFs(cluster *driver.Cluster) {
	// create and export a variable RUN_TEST_FILE_REMOVAL_SAFETY="true" to test the safety removal of files
	serverIp := cluster.ServerIPs[0]
	exportErr := resources.ExportEnvProfileNode(
		[]string{serverIp},
		map[string]string{"RUN_TEST_FILE_REMOVAL_SAFETY": "true"},
		"",
	)
	Expect(exportErr).NotTo(HaveOccurred(), "failed to export RUN_TEST_FILE_REMOVAL_SAFETY variable to server nodes %v",
		exportErr)

	// create fake remote fs on server under kubelet on the first server node.
	createTestPodDir := "sudo mkdir -p /var/lib/kubelet/data/test-pod/volumes/test-mount"
	ures, err := resources.RunCommandOnNode(createTestPodDir, serverIp)
	Expect(err).NotTo(HaveOccurred(), "failed to create test mount dir on server")
	Expect(ures).To(BeEmpty(), "failed to create test mount dir on server")

	createTmpfsDir := "sudo mkdir -p /mnt/fake-remote-fs"
	ures, err = resources.RunCommandOnNode(createTmpfsDir, serverIp)
	Expect(err).NotTo(HaveOccurred(), "failed to create tmpfs mount point on server")
	Expect(ures).To(BeEmpty(), "failed to create tmpfs mount point on server")

	mountTmpfs := "sudo mount -t tmpfs -o size=100M tmpfs /mnt/fake-remote-fs"
	ures, err = resources.RunCommandOnNode(mountTmpfs, serverIp)
	Expect(err).NotTo(HaveOccurred(), "failed to mount tmpfs on server")
	Expect(ures).To(BeEmpty(), "failed to mount tmpfs on server")

	// Create test files in the tmpfs filesystem.
	createTestFile := "echo 'Important remote data - DO NOT DELETE' | sudo tee /mnt/fake-remote-fs/important.txt"
	ures, err = resources.RunCommandOnNode(createTestFile, serverIp)
	Expect(err).NotTo(HaveOccurred(), "failed to create important.txt file in fake remote fs on server")
	Expect(ures).To(ContainSubstring("DO NOT DELETE"),
		"failed to create important.txt file in fake remote fs on server")

	createTestFile = "echo 'Critical file' | sudo tee /mnt/fake-remote-fs/critical.txt"
	ures, err = resources.RunCommandOnNode(createTestFile, serverIp)
	Expect(err).NotTo(HaveOccurred(), "failed to create critical.txt file in fake remote fs on server")
	Expect(ures).To(ContainSubstring("Critical file"),
		"failed to create critical.txt file in fake remote fs on server")

	mountDir := "sudo mount --bind /mnt/fake-remote-fs /var/lib/kubelet/data/test-pod/volumes/test-mount"
	ures, err = resources.RunCommandOnNode(mountDir, serverIp)
	Expect(err).NotTo(HaveOccurred(), "failed to mount fake remote fs on server")
	Expect(ures).To(BeEmpty(), "failed to mount fake remote fs on server")
}
