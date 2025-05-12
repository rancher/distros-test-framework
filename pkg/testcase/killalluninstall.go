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
	killAllTimeout = 25 * time.Second

	errDirNotFound   = " no such file or directory"
	errNoMount       = " no mount point specified"
	errMount         = " mount point does not exist"
	errMountNotFound = " not found"
	errNotMounted    = " not mounted"
)

func TestKillAllUninstall(cluster *shared.Cluster, cfg *config.Env) {
	productDataDir := "/var/lib/rancher/" + cluster.Config.Product

	// exporting binary directories to all nodes, so when script test runs, it can already have the paths needed.
	dirErr := exportBinDirs(
		append(cluster.ServerIPs, cluster.AgentIPs...),
		"crictl",
		"kubectl",
		"ctr",
	)
	Expect(dirErr).NotTo(HaveOccurred(), "failed to get binary directories: %v", dirErr)

	scpTestScripts(cluster)

	mountBindDataDir(cluster, productDataDir)

	killAllValidations(cluster, "true", true)

	shared.LogLevel("info", "umounting data dir: %s since kill all after v1.30.4 doesn't unmount if previously mounted",
		productDataDir)
	umountDataDir(cluster, productDataDir)

	shared.LogLevel("info", "running uninstall script and tests after kill all")
	uninstallValidations(cluster, true)

	shared.LogLevel("info", "reinstalling product after uninstall")
	reInstallProduct(cluster, cfg)

	shared.LogLevel("info", "running kill all and uninstall tests without mounting data dir after reinstall")
	killAllValidations(cluster, "false", false)
	uninstallValidations(cluster, false)

	shared.LogLevel("info", "reinstalling again to test uninstall script alone")
	reInstallProduct(cluster, cfg)

	shared.LogLevel("info", "running uninstall tests")
	uninstallValidations(cluster, false)
}

// exportBinDirs finds and exports binary paths to all specified nodes.
func exportBinDirs(ips []string, binaries ...string) error {
	if len(ips) == 0 {
		return errors.New("need at least one IP address")
	}

	// ips[0] to only use one node to find the binaries.
	binPaths, err := shared.FindBinaries(ips[0], binaries...)
	if err != nil {
		return fmt.Errorf("failed to find binaries: %w", err)
	}

	envVars := make(map[string]string)
	for bin, dir := range binPaths {
		varName := strings.ToUpper(bin) + "_BIN_DIR"
		envVars[varName] = dir
	}

	exportErr := shared.ExportEnvProfileNode(ips, envVars, "bin_paths.sh")
	if exportErr != nil {
		return fmt.Errorf("failed to export environment variables: %w", exportErr)
	}

	return nil
}

func mountBindDataDir(cluster *shared.Cluster, productDataDir string) {
	err := shared.MountBind(cluster.ServerIPs, productDataDir+"/server", productDataDir+"/server")
	if err != nil {
		if strings.Contains(err.Error(), " mount point does not exist") {
			shared.LogLevel("info", "data dir not mounted on server, mount point does not exist: %v", err)
		} else {
			Expect(err).NotTo(HaveOccurred(), "failed to mount bind server data dir for server on server nodes")
		}
	}

	// mount bind for agent iPs.
	err = shared.MountBind(cluster.AgentIPs, productDataDir+"/agent", productDataDir+"/agent")
	if err != nil {
		if strings.Contains(err.Error(), " mount point does not exist") {
			shared.LogLevel("info", "data dir not mounted on agent, mount point does not exist: %v", err)
		} else {
			Expect(err).NotTo(HaveOccurred(), "failed to mount bind agent data dir for agent on agent nodes")
		}
	}
}

func scpTestScripts(cluster *shared.Cluster) {
	// scp test kill all script.
	killAllLocalPath := shared.BasePath() + "/scripts/kill-all_test.sh"
	killAllRemotePath := "/var/tmp/kill-all_test.sh"
	scpErr := shared.RunScp(cluster, cluster.ServerIPs[0], []string{killAllLocalPath}, []string{killAllRemotePath})
	Expect(scpErr).NotTo(HaveOccurred(), "failed to scp kill all test script to server")

	// scp test kill all scrip to agent.
	scpAgentErr := shared.RunScp(cluster, cluster.AgentIPs[0], []string{killAllLocalPath}, []string{killAllRemotePath})
	Expect(scpAgentErr).NotTo(HaveOccurred(), "failed to scp kill all test script to agent")

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

	// scp test uninstall script to agent.
	scpUninstallAgentErr := shared.RunScp(
		cluster,
		cluster.AgentIPs[0],
		[]string{uninstallLocalPath},
		[]string{uninstallRemotePath},
	)
	Expect(scpUninstallAgentErr).NotTo(HaveOccurred(), "failed to scp uninstall test script to agent")
}

func umountDataDir(cluster *shared.Cluster, productDataDir string) {
	umountAllProductMounts(productDataDir, cluster.ServerIPs[0], "server")
	umountAllProductMounts(productDataDir, cluster.AgentIPs[0], "agent")
}

func umountAllProductMounts(productDataDir, nodeIP, nodeType string) {
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
	waitScriptFinish := time.After(killAllTimeout)
	<-waitScriptFinish

	killTesCmd := "sudo bash /var/tmp/kill-all_test.sh -mount " + mount
	res, runKillAllErr := shared.RunCommandOnNode(killTesCmd, cluster.ServerIPs[0])
	Expect(runKillAllErr).NotTo(HaveOccurred(), "failed to run kill all test script on server: %v", runKillAllErr)
	Expect(strings.TrimSpace(res)).To(ContainSubstring(
		"All killall operations were successful!"),
		"failed to run kill all test script on server")

	if agent {
		killAllAgentErr := shared.ManageProductCleanup(cluster.Config.Product, "agent", cluster.AgentIPs[0], "killall")
		Expect(killAllAgentErr).NotTo(HaveOccurred(), "failed to run killall for product: %v agent", cluster.Config.Product)

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

	waitScriptFinish := time.After(30 * time.Second)
	<-waitScriptFinish

	uninstallTestCmd := "sudo bash /var/tmp/uninstall_test.sh -p " + cluster.Config.Product

	res, uninstallCmdErr := shared.RunCommandOnNode(uninstallTestCmd, cluster.ServerIPs[0])
	Expect(uninstallCmdErr).NotTo(HaveOccurred(), "failed to run uninstall test script on server")
	Expect(strings.TrimSpace(res)).To(ContainSubstring(
		"All uninstall operations were successful!"),
		"failed to run uninstall test script on server")

	if agent {
		uninstalAgentErr := shared.ManageProductCleanup(cluster.Config.Product, "agent", cluster.AgentIPs[0], "uninstall")
		Expect(uninstalAgentErr).NotTo(HaveOccurred(),
			"failed to run uninstall for product: %v agent",
			cluster.Config.Product)

		res, uninstallCmdErr = shared.RunCommandOnNode(uninstallTestCmd, cluster.AgentIPs[0])
		Expect(uninstallCmdErr).NotTo(HaveOccurred(), "failed to run uninstall test script on agent")
		Expect(strings.TrimSpace(res)).To(ContainSubstring(
			"All uninstall operations were successful!"),
			"failed to run uninstall test script on agent")
	}

	shared.LogLevel("info", "uninstall test script went through successfully")
}

func reInstallProduct(cluster *shared.Cluster, cfg *config.Env) {
	installErr := shared.InstallProduct(cluster, cluster.ServerIPs[0], cfg.InstallVersion)
	Expect(installErr).NotTo(HaveOccurred(), "failed to install product: %v", installErr)

	enableErr := shared.EnableAndStartService(cluster, cluster.ServerIPs[0], "server")
	Expect(enableErr).NotTo(HaveOccurred(), "failed to enable and start service: %v", enableErr)
}
