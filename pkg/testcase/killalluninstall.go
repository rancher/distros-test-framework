package testcase

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

const (
	errDirNotFound   = " no such file or directory"
	errNoMount       = " no mount point specified"
	errMount         = " mount point does not exist"
	errMountNotFound = " not found"
	errNotMounted    = " not mounted"
)

func TestKillAllUninstall(cluster *shared.Cluster, cfg *config.Env) {
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

	shared.LogLevel("info", "umounting data dir: %s since kill all after v1.30.4 doesn't unmount if previously mounted",
		productDataDir)
	umountDataDir(cluster, productDataDir)

	shared.LogLevel("info", "running uninstall script and tests after kill all already ran")
	uninstallValidations(cluster, true)

	shared.LogLevel("info", "reinstalling product after uninstall")
	reInstallServerProduct(cluster, cfg)

	shared.LogLevel("info", "running uninstall tests without mounting any data dir after reinstall")
	uninstallValidations(cluster, false)

	shared.LogLevel("info", "reinstalling again to test uninstall script alone with file removal safety")
	reInstallServerProduct(cluster, cfg)

	// for now only rke2 will support this test.
	// since now we needed to mount a fake remote fs, we will handle uninstall tests based on that, because
	// the uninstall script will try to remove the kubelet dir and return device or resource busy error.
	// ( this is expected and handled in the test script )
	if cluster.Config.Product == "rke2" {
		createFakeRemoteFs(cluster)
	}

	uninstallValidations(cluster, false)
}

// exportBinDirs finds and exports binary paths to all specified nodes.
func exportBinDirs(ip string, binaries ...string) error {
	if ip == "" {
		return errors.New("need at least one IP address")
	}

	// ips[0] to only use one node to find the binaries.
	binPaths, err := shared.FindBinaries(ip, binaries...)
	if err != nil {
		return fmt.Errorf("failed to find binaries: %w", err)
	}

	envVars := make(map[string]string)
	for bin, dir := range binPaths {
		varName := strings.ToUpper(bin) + "_BIN_DIR"
		envVars[varName] = dir
	}

	exportErr := shared.ExportEnvProfileNode([]string{ip}, envVars, "bin_paths.sh")
	if exportErr != nil {
		return fmt.Errorf("failed to export environment variables: %w", exportErr)
	}

	return nil
}

func scpTestScripts(cluster *shared.Cluster) {
	// scp test kill all script.
	killAllLocalPath := shared.BasePath() + "/scripts/kill-all_test.sh"
	killAllRemotePath := "/var/tmp/kill-all_test.sh"
	scpErr := shared.RunScp(cluster, cluster.ServerIPs[0], []string{killAllLocalPath}, []string{killAllRemotePath})
	Expect(scpErr).NotTo(HaveOccurred(), "failed to scp kill all test script to server")

	// scp test kill all scrip to agent if any.
	if len(cluster.AgentIPs) > 0 {
		scpAgentErr := shared.RunScp(cluster, cluster.AgentIPs[0], []string{killAllLocalPath}, []string{killAllRemotePath})
		Expect(scpAgentErr).NotTo(HaveOccurred(), "failed to scp kill all test script to agent")
	}

	// scp test uninstal script to server.
	uninstallLocalPath := shared.BasePath() + "/scripts/uninstall_test.sh"
	uninstallRemotePath := "/var/tmp/uninstall_test.sh"
	scpUninstallErr := shared.RunScp(
		cluster,
		cluster.ServerIPs[0],
		[]string{uninstallLocalPath},
		[]string{uninstallRemotePath},
	)
	Expect(scpUninstallErr).NotTo(HaveOccurred(), "failed to scp uninstall test script to server")

	// scp test uninstall script to agent if any.
	if len(cluster.AgentIPs) > 0 {
		scpUninstallAgentErr := shared.RunScp(
			cluster,
			cluster.AgentIPs[0],
			[]string{uninstallLocalPath},
			[]string{uninstallRemotePath},
		)
		Expect(scpUninstallAgentErr).NotTo(HaveOccurred(), "failed to scp uninstall test script to agent")
	}
}

func mountBindDataDir(cluster *shared.Cluster, productDataDir string) {
	err := shared.MountBind([]string{cluster.ServerIPs[0]}, productDataDir+"/server", productDataDir+"/server")
	if err != nil {
		if strings.Contains(err.Error(), " mount point does not exist") {
			shared.LogLevel("info", "data dir not mounted on server, mount point does not exist: %v", err)
		} else {
			Expect(err).NotTo(HaveOccurred(), "failed to mount bind server data dir for server on server nodes")
		}
	}

	// mount bind for agent IPs if any.
	if len(cluster.AgentIPs) > 0 {
		err = shared.MountBind(cluster.AgentIPs, productDataDir+"/agent", productDataDir+"/agent")
		if err != nil {
			if strings.Contains(err.Error(), " mount point does not exist") {
				shared.LogLevel("info", "data dir not mounted on agent, mount point does not exist: %v", err)
			} else {
				Expect(err).NotTo(HaveOccurred(), "failed to mount bind agent data dir for agent on agent nodes")
			}
		}
	}
}

func umountDataDir(cluster *shared.Cluster, productDataDir string) {
	umountAllProductDir(productDataDir, cluster.ServerIPs[0], "server")

	if len(cluster.AgentIPs) > 0 {
		umountAllProductDir(productDataDir, cluster.AgentIPs[0], "agent")
	}
}

func umountAllProductDir(productDataDir, nodeIP, nodeType string) {
	forceUmountCmd := "sudo umount -f -R " + productDataDir + "/server/" + " && " +
		"sudo umount -f -R " + productDataDir + "/agent/"

	if nodeType == "agent" {
		forceUmountCmd = "sudo umount -f -R " + productDataDir + "/agent/"
	}
	ures, err := shared.RunCommandOnNode(forceUmountCmd, nodeIP)
	if err != nil {
		var exitError *ssh.ExitError
		if errors.As(err, &exitError) {
			errMsg := exitError.Msg()
			if strings.Contains(errMsg, errNotMounted) ||
				strings.Contains(errMsg, errMount) ||
				strings.Contains(errMsg, errNoMount) ||
				strings.Contains(errMsg, errDirNotFound) ||
				strings.Contains(errMsg, errMountNotFound) {
				shared.LogLevel("warn", "not mounted %s on node %s, mount point does not exist: %v",
					productDataDir, nodeIP, err)
			}
		}
	} else {
		Expect(err).NotTo(HaveOccurred(),
			"failed to umount on node %s: %v", nodeIP, err)
	}
	Expect(ures).To(BeEmpty(), "failed to umount on node %s", nodeIP)

	findMountsCmd := "find " + productDataDir + " -path '*/containerd/tmpmounts/containerd-mount*' " +
		" -type d 2>/dev/null || true"
	path, _ := shared.RunCommandOnNode(findMountsCmd, nodeIP)
	if path != "" {
		path = strings.TrimSpace(path)
		shared.LogLevel("info", "umounting all containerd mounts on node %s: %s", nodeIP, path)

		umountCmd := "sudo umount -f " + path + " 2>/dev/null || sudo umount -l " + path + " 2>/dev/null || true"
		ures, err = shared.RunCommandOnNode(umountCmd, nodeIP)
		if err != nil {
			var exitError *ssh.ExitError
			if errors.As(err, &exitError) {
				errMsg := exitError.Msg()
				if strings.Contains(errMsg, errNotMounted) ||
					strings.Contains(errMsg, errMount) ||
					strings.Contains(errMsg, errNoMount) ||
					strings.Contains(errMsg, errDirNotFound) ||
					strings.Contains(errMsg, errMountNotFound) {
					shared.LogLevel("warn", "not mounted %s on node %s, mount point does not exist: %v",
						productDataDir+"/agent/tmpmounts", nodeIP, errMsg)
				}
			} else {
				Expect(err).NotTo(HaveOccurred(),
					"failed to umount on node %s: %v", nodeIP, err)
			}
		}
	}
	Expect(ures).To(BeEmpty(), "failed to umount %s on node %s", productDataDir+"/tmp", nodeIP)
}

func killAllValidations(cluster *shared.Cluster, mount string, agent bool) {
	killAllErr := shared.ManageProductCleanup(cluster.Config.Product, "server", cluster.ServerIPs[0], "killall")
	Expect(killAllErr).NotTo(HaveOccurred(), "failed to run killall for product: %v server", cluster.Config.Product)

	// Wait for killall process to complete
	killallPattern := fmt.Sprintf("%s.*killall|killall.*%s", cluster.Config.Product, cluster.Config.Product)
	waitErr := shared.CheckProcessCompletion(cluster.ServerIPs[0], killallPattern, 10, 10*time.Second)
	Expect(waitErr).NotTo(HaveOccurred(), "failed waiting for killall process to complete: %v", waitErr)

	killTesCmd := "sudo bash /var/tmp/kill-all_test.sh -mount " + mount
	res, runKillAllErr := shared.RunCommandOnNode(killTesCmd, cluster.ServerIPs[0])
	Expect(runKillAllErr).NotTo(HaveOccurred(), "failed to run kill all test script on server: %v", runKillAllErr)
	Expect(strings.TrimSpace(res)).To(ContainSubstring(
		"All killall operations were successful!"),
		"failed to run kill all test script on server")

	shared.LogLevel("debug", "kill all test script output on server: %s", res)

	if agent && len(cluster.AgentIPs) > 0 {
		killAllAgentErr := shared.ManageProductCleanup(cluster.Config.Product, "agent", cluster.AgentIPs[0], "killall")
		Expect(killAllAgentErr).NotTo(HaveOccurred(), "failed to run killall for product: %v agent", cluster.Config.Product)

		// Wait for agent killall process to complete
		waitAgentErr := shared.CheckProcessCompletion(cluster.AgentIPs[0], killallPattern, 10, 10*time.Second)
		Expect(waitAgentErr).NotTo(HaveOccurred(), "failed waiting for agent killall process to complete: %v", waitAgentErr)

		res, runKillAllErr = shared.RunCommandOnNode(killTesCmd, cluster.AgentIPs[0])
		Expect(runKillAllErr).NotTo(HaveOccurred(),
			"failed to run kill all test script on agent: %v", runKillAllErr)
		Expect(strings.TrimSpace(res)).To(ContainSubstring(
			"All killall operations were successful!"),
			"failed to run kill all test script on agent")
	}

	shared.LogLevel("info", "kill all test script went through successfully with mount: %s", mount)
}

func uninstallValidations(cluster *shared.Cluster, agent bool) {
	uninstalErr := shared.ManageProductCleanup(cluster.Config.Product, "server", cluster.ServerIPs[0], "uninstall")
	Expect(uninstalErr).NotTo(HaveOccurred(), "failed to run uninstall for product: %v server", cluster.Config.Product)

	// Wait for uninstall process to complete
	uninstallPattern := fmt.Sprintf("%s.*uninstall|uninstall.*%s", cluster.Config.Product, cluster.Config.Product)
	waitErr := shared.CheckProcessCompletion(cluster.ServerIPs[0], uninstallPattern, 10, 10*time.Second)
	Expect(waitErr).NotTo(HaveOccurred(), "failed waiting for uninstall process to complete: %v", waitErr)

	uninstallTestCmd := "sudo bash /var/tmp/uninstall_test.sh -p " + cluster.Config.Product
	res, uninstallCmdErr := shared.RunCommandOnNode(uninstallTestCmd, cluster.ServerIPs[0])
	Expect(uninstallCmdErr).NotTo(HaveOccurred(), "failed to run uninstall test script on server")
	Expect(strings.TrimSpace(res)).To(ContainSubstring(
		"All uninstall operations were successful!"),
		"failed to run uninstall test script on server")

	shared.LogLevel("debug", "uninstall test script output on server: %s", res)

	if agent && len(cluster.AgentIPs) > 0 {
		uninstalAgentErr := shared.ManageProductCleanup(cluster.Config.Product, "agent", cluster.AgentIPs[0], "uninstall")
		Expect(uninstalAgentErr).NotTo(HaveOccurred(),
			"failed to run uninstall for product: %v agent",
			cluster.Config.Product)

		waitAgentErr := shared.CheckProcessCompletion(cluster.AgentIPs[0], uninstallPattern, 10, 10*time.Second)
		Expect(waitAgentErr).NotTo(HaveOccurred(), "failed waiting for agent uninstall process to complete: %v",
			waitAgentErr)

		res, uninstallCmdErr = shared.RunCommandOnNode(uninstallTestCmd, cluster.AgentIPs[0])
		Expect(uninstallCmdErr).NotTo(HaveOccurred(), "failed to run uninstall test script on agent")
		Expect(strings.TrimSpace(res)).To(ContainSubstring(
			"All uninstall operations were successful!"),
			"failed to run uninstall test script on agent")
	}

	shared.LogLevel("info", "uninstall test script went through successfully")
}

// reInstallServerProduct reinstalls the product on the first server node only.
func reInstallServerProduct(cluster *shared.Cluster, cfg *config.Env) {
	installErr := shared.InstallProduct(cluster, cluster.ServerIPs[0], cfg.InstallVersion)
	Expect(installErr).NotTo(HaveOccurred(), "failed to install product: %v", installErr)

	enableErr := shared.EnableAndStartService(cluster, cluster.ServerIPs[0], "server")
	Expect(enableErr).NotTo(HaveOccurred(), "failed to enable and start service: %v", enableErr)
}

func createFakeRemoteFs(cluster *shared.Cluster) {
	// create and export a variable RUN_TEST_FILE_REMOVAL_SAFETY="true" to test the safety removal of files
	serverIp := cluster.ServerIPs[0]
	exportErr := shared.ExportEnvProfileNode(
		[]string{serverIp},
		map[string]string{"RUN_TEST_FILE_REMOVAL_SAFETY": "true"},
		"",
	)
	Expect(exportErr).NotTo(HaveOccurred(), "failed to export RUN_TEST_FILE_REMOVAL_SAFETY variable to server nodes %v",
		exportErr)

	// create fake remote fs on server under kubelet on the first server node.
	createTestPodDir := "sudo mkdir -p /var/lib/kubelet/data/test-pod/volumes/test-mount"
	ures, err := shared.RunCommandOnNode(createTestPodDir, serverIp)
	Expect(err).NotTo(HaveOccurred(), "failed to create test mount dir on server")
	Expect(ures).To(BeEmpty(), "failed to create test mount dir on server")

	createTmpfsDir := "sudo mkdir -p /mnt/fake-remote-fs"
	ures, err = shared.RunCommandOnNode(createTmpfsDir, serverIp)
	Expect(err).NotTo(HaveOccurred(), "failed to create tmpfs mount point on server")
	Expect(ures).To(BeEmpty(), "failed to create tmpfs mount point on server")

	mountTmpfs := "sudo mount -t tmpfs -o size=100M tmpfs /mnt/fake-remote-fs"
	ures, err = shared.RunCommandOnNode(mountTmpfs, serverIp)
	Expect(err).NotTo(HaveOccurred(), "failed to mount tmpfs on server")
	Expect(ures).To(BeEmpty(), "failed to mount tmpfs on server")

	// Create test files in the tmpfs filesystem.
	createTestFile := "echo 'Important remote data - DO NOT DELETE' | sudo tee /mnt/fake-remote-fs/important.txt"
	ures, err = shared.RunCommandOnNode(createTestFile, serverIp)
	Expect(err).NotTo(HaveOccurred(), "failed to create important.txt file in fake remote fs on server")
	Expect(ures).To(ContainSubstring("DO NOT DELETE"),
		"failed to create important.txt file in fake remote fs on server")

	createTestFile = "echo 'Critical file' | sudo tee /mnt/fake-remote-fs/critical.txt"
	ures, err = shared.RunCommandOnNode(createTestFile, serverIp)
	Expect(err).NotTo(HaveOccurred(), "failed to create critical.txt file in fake remote fs on server")
	Expect(ures).To(ContainSubstring("Critical file"),
		"failed to create critical.txt file in fake remote fs on server")

	mountDir := "sudo mount --bind /mnt/fake-remote-fs /var/lib/kubelet/data/test-pod/volumes/test-mount"
	ures, err = shared.RunCommandOnNode(mountDir, serverIp)
	Expect(err).NotTo(HaveOccurred(), "failed to mount fake remote fs on server")
	Expect(ures).To(BeEmpty(), "failed to mount fake remote fs on server")
}
