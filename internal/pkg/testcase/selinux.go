package testcase

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

type cmdCtx map[string]string

type configuration struct {
	distroName string
	cmdCtx
}

// TestSelinuxEnabled Validates that containerd is running with selinux enabled in the config.
func TestSelinuxEnabled(cluster *resources.Cluster) {
	ips := cluster.ServerIPs

	for _, ip := range ips {
		err := resources.VerifyFileContent("/etc/rancher/"+cluster.Config.Product+"/config.yaml", "selinux: true", ip)
		Expect(err).NotTo(HaveOccurred())

		filePath := fmt.Sprintf("/var/lib/rancher/%s/agent/etc/containerd/config.toml", cluster.Config.Product)
		errCont := resources.VerifyFileContent(filePath, "enable_selinux = true", ip)
		Expect(errCont).NotTo(HaveOccurred())
	}
}

// TestSelinux Validates container-selinux version, rke2-selinux version and rke2-selinux version.
func TestSelinux(cluster *resources.Cluster) {
	serverCmd := "rpm -qa container-selinux rke2-server rke2-selinux"
	serverAsserts := []string{"container-selinux", "rke2-selinux", "rke2-server"}
	agentAsserts := []string{"container-selinux", cluster.Config.Product + "-selinux"}

	if cluster.Config.Product == "k3s" {
		serverCmd = "rpm -qa container-selinux k3s-selinux"
		serverAsserts = []string{"container-selinux", "k3s-selinux"}
	}

	if cluster.NumServers > 0 {
		for _, serverIP := range cluster.ServerIPs {
			err := assert.CheckComponentCmdNode(serverCmd, serverIP, serverAsserts...)
			Expect(err).NotTo(HaveOccurred())
		}
	}

	if cluster.NumAgents > 0 {
		for _, agentIP := range cluster.AgentIPs {
			err := assert.CheckComponentCmdNode(
				"rpm -qa container-selinux "+cluster.Config.Product+"-selinux",
				agentIP,
				agentAsserts...,
			)
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

func getVersion(osRelease, ip string) (string, error) {
	if !strings.Contains(osRelease, "VERSION_ID") {
		return "", resources.ReturnLogError("VERSION_ID not found in: %s", osRelease)
	}
	res, err := resources.RunCommandOnNode("cat /etc/os-release | grep 'VERSION_ID'", ip)
	Expect(err).NotTo(HaveOccurred())

	parts := strings.Split(res, "=")
	if len(parts) != 2 {
		return "", resources.ReturnLogError("unexpected format for VERSION_ID")
	}
	version := strings.Trim(parts[1], "\"")
	if dotIndex := strings.Index(version, "."); dotIndex != -1 {
		version = version[:dotIndex]
	}

	return version, nil
}

var osPolicy string

func getContext(product, ip string) (cmdCtx, error) {
	res, err := resources.RunCommandOnNode("cat /etc/os-release", ip)
	if err != nil {
		return nil, err
	}

	fmt.Println("OS Release: \n", res)
	policyMapping := map[string]string{
		"ID_LIKE='suse' VARIANT_ID='sle-micro'": "sle_micro",
		"ID_LIKE='suse'":                        "micro_os",
		"ID_LIKE='coreos'":                      "coreos",
		"VARIANT_ID='coreos'":                   "coreos",
	}

	for k, v := range policyMapping {
		if strings.Contains(res, k) {
			return selectSelinuxPolicy(product, v), nil
		}
	}

	version, err := getVersion(res, ip)
	if err != nil {
		return nil, resources.ReturnLogError("failed to get version: %v", err)
	}
	if version == "" {
		return nil, resources.ReturnLogError("could not determine version for os: %s", res)
	}

	versionMapping := map[string]string{
		"7": "centos7",
		"8": "centos8",
		"9": "centos9",
	}

	if policy, ok := versionMapping[version]; ok {
		return selectSelinuxPolicy(product, policy), nil
	}

	return nil, fmt.Errorf("unable to determine policy for %s on os: %s", ip, res)
}

func selectSelinuxPolicy(product, osType string) cmdCtx {
	key := fmt.Sprintf("%s_%s", product, osType)

	for _, config := range conf {
		if config.distroName == key {
			fmt.Printf("\nUsing '%s' policy for this %s cluster.\n", osType, product)
			osPolicy = osType
			return config.cmdCtx
		}
	}

	fmt.Printf("Configuration for %s not found!\n", key)

	return nil
}

// TestSelinuxSpcT Validate that containers don't run with spc_t.
func TestSelinuxSpcT(cluster *resources.Cluster) {
	for _, serverIP := range cluster.ServerIPs {
		// removing err here since this is actually returning exit 1.
		res, _ := resources.RunCommandOnNode("ps auxZ | grep metrics | grep -v grep", serverIP)
		Expect(res).ShouldNot(ContainSubstring("spc_t"))
	}
}

// TestUninstallPolicy Validate that un-installation will remove the rke2-selinux or k3s-selinux policy.
// Call this function after the un-installation of the product.
func TestUninstallPolicy(cluster *resources.Cluster, uninstall bool) {
	serverCmd := "rpm -qa container-selinux rke2-server rke2-selinux"
	if cluster.Config.Product == "k3s" {
		serverCmd = "rpm -qa container-selinux k3s-selinux"
	}

	for _, serverIP := range cluster.ServerIPs {
		if uninstall {
			resources.LogLevel("info", "Uninstalling %s on server: %s", cluster.Config.Product, serverIP)
			err := resources.ManageProductCleanup(cluster.Config.Product, "server", serverIP, "uninstall")
			if err != nil {
				if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "failed to find") {
					resources.LogLevel("info", "Product %s already uninstalled on server: %s", cluster.Config.Product, serverIP)
					continue
				} else {
					resources.LogLevel("error", "Failed to uninstall %s on server: %s, error: %v", cluster.Config.Product, serverIP, err)
					Expect(err).NotTo(HaveOccurred(), "Failed to uninstall %s on server: %s", cluster.Config.Product, serverIP)
				}
			}
		}

		verifyUninstallPolicy(cluster.Config.Product, serverIP, serverCmd)
	}

	for _, agentIP := range cluster.AgentIPs {
		if uninstall {
			resources.LogLevel("info", "Uninstalling %s on agent: %s", cluster.Config.Product, agentIP)
			err := resources.ManageProductCleanup(cluster.Config.Product, "agent", agentIP, "uninstall")
			if err != nil {
				if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "failed to find") {
					resources.LogLevel("info", "Product %s already uninstalled on agent: %s", cluster.Config.Product, agentIP)
					continue
				} else {
					resources.LogLevel("error", "Failed to uninstall %s on agent: %s, error: %v", cluster.Config.Product, agentIP, err)
					Expect(err).NotTo(HaveOccurred(), "Failed to uninstall %s on agent: %s", cluster.Config.Product, agentIP)
				}
			}
		}

		cmd := "rpm -qa container-selinux " + cluster.Config.Product + "-selinux"
		verifyUninstallPolicy(cluster.Config.Product, agentIP, cmd)
	}
}

func verifyUninstallPolicy(product, ip, cmd string) {
	res, err := resources.RunCommandOnNode(cmd, ip)
	Expect(err).NotTo(HaveOccurred())

	if strings.Contains(osPolicy, "centos7") {
		Expect(res).Should(ContainSubstring("container-selinux"))
		Expect(res).ShouldNot(ContainSubstring(product + "-selinux"))
	} else {
		Expect(res).Should(BeEmpty())
	}
}

// https://github.com/k3s-io/k3s/blob/master/install.sh.
// https://github.com/rancher/rke2/blob/master/install.sh.
// Based on this info, this is the way to validate the correct context.

// TestSelinuxContext Validates directories to ensure they have the correct selinux contexts created.
func TestSelinuxContext(cluster *resources.Cluster) {
	var err error

	if cluster.NumServers > 0 {
		for _, ip := range cluster.ServerIPs {
			var context map[string]string
			context, err = getContext(cluster.Config.Product, ip)
			Expect(err).NotTo(HaveOccurred())

			var res string
			for cmd, expectedContext := range context {
				res, err = resources.RunCommandOnNode(cmd, ip)
				fmt.Printf("\nCommand:\n%s \nContext expected:\n%s\nResult:\n%s\n", cmd, expectedContext, res)
				if res != "" {
					Expect(res).Should(ContainSubstring(expectedContext),
						"error on cmd %v \n Context %v \nnot found on ", cmd, expectedContext, res)
					Expect(err).NotTo(HaveOccurred())
				}
			}
		}
	}
}

var (
	cmdPrefix  = "sudo ls -laZ"
	ignoreDir  = "-I .. -I ."
	rke2       = "/var/lib/rancher/rke2"
	k3s        = "/var/lib/rancher/k3s"
	systemD    = "/etc/systemd/system"
	usrBin     = "/usr/bin"
	usrLocal   = "/usr/local/bin"
	grepFilter = "| grep -v \"/\""
)

const (
	ctxUnitFile = "system_u:object_r:container_unit_file_t:s0"
	ctxExec     = "system_u:object_r:container_runtime_exec_t:s0"
	ctxVarLib   = "system_u:object_r:container_var_lib_t:s0"
	ctxFile     = "system_u:object_r:container_file_t:s0"
	ctxConfig   = "system_u:object_r:container_config_t:s0"
	ctxShare    = "system_u:object_r:container_share_t:s0"
	ctxRoFile   = "system_u:object_r:container_ro_file_t:s0"
	ctxLog      = "system_u:object_r:container_log_t:s0"
	ctxRunTmpfs = "system_u:object_r:container_var_run_t:s0"
	ctxTmpfs    = "system_u:object_r:container_runtime_tmpfs_t:s0"
	ctxTLS      = "system_u:object_r:rke2_tls_t:s0"
	ctxLock     = "system_u:object_r:k3s_lock_t:s0"
	ctxData     = "system_u:object_r:k3s_data_t:s0"
	ctxRoot     = "system_u:object_r:k3s_root_t:s0"
	ctxNone     = "<<none>>"
	ctxRke2TLS  = "system_u:object_r:rke2_tls_t:s0"
)

//nolint:dupl // this is expected.
var conf = []configuration{
	{
		distroName: "rke2_centos7",
		cmdCtx: cmdCtx{
			cmdPrefix + " " + systemD + "/rke2*":                                                            ctxUnitFile,
			cmdPrefix + " " + "/lib" + systemD + "/rke2*":                                                   ctxUnitFile,
			cmdPrefix + " " + usrLocal + "/lib" + systemD + "/rke2*":                                        ctxUnitFile,
			cmdPrefix + " " + usrBin + "/rke2":                                                              ctxExec,
			cmdPrefix + " " + usrLocal + "/rke2":                                                            ctxExec,
			cmdPrefix + " " + "/var/lib/cni " + ignoreDir:                                                   ctxVarLib,
			cmdPrefix + " " + "/var/lib/cni/* " + ignoreDir:                                                 ctxVarLib,
			cmdPrefix + " " + "/opt/cni " + ignoreDir:                                                       ctxFile,
			cmdPrefix + " " + "/opt/cni/* " + ignoreDir:                                                     ctxFile,
			cmdPrefix + " " + "/var/lib/kubelet/pods " + ignoreDir:                                          ctxFile,
			cmdPrefix + " " + "/var/lib/kubelet/pods/* " + ignoreDir:                                        ctxFile,
			cmdPrefix + " " + rke2 + " " + ignoreDir:                                                        ctxVarLib,
			cmdPrefix + " " + rke2 + "/* " + ignoreDir:                                                      ctxVarLib,
			cmdPrefix + " " + rke2 + "/data":                                                                ctxExec,
			cmdPrefix + " " + rke2 + "/data/*":                                                              ctxExec,
			cmdPrefix + " " + rke2 + "/data/*/charts " + ignoreDir + " " + grepFilter:                       ctxConfig,
			cmdPrefix + " " + rke2 + "/data/*/charts/* " + ignoreDir + " " + grepFilter:                     ctxConfig,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots " + ignoreDir + " " + grepFilter:        ctxShare,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots/* " + ignoreDir + " " + grepFilter:      ctxShare,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots/*/.* " + " " + grepFilter:               ctxNone,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/sandboxes " + ignoreDir + " " + grepFilter:        ctxShare,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/sandboxes/* " + ignoreDir + " " + grepFilter:      ctxShare,
			cmdPrefix + " " + rke2 + "/server/logs " + ignoreDir:                                            ctxLog,
			cmdPrefix + " " + rke2 + "/server/logs/ " + ignoreDir:                                           ctxLog,
			cmdPrefix + " " + "/var/run/flannel " + ignoreDir:                                               ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/flannel/* " + ignoreDir:                                             ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s " + ignoreDir:                                                   ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/* " + ignoreDir:                                                 ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm " + ignoreDir + " " + grepFilter:   ctxTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm/* " + ignoreDir + " " + grepFilter: ctxTmpfs,
			cmdPrefix + " " + "/var/log/containers " + ignoreDir:                                            ctxLog,
			cmdPrefix + " " + "/var/log/containers/* " + ignoreDir:                                          ctxLog,
			cmdPrefix + " " + "/var/log/pods " + ignoreDir:                                                  ctxLog,
			cmdPrefix + " " + "/var/log/pods/* " + ignoreDir:                                                ctxLog,
			cmdPrefix + " " + rke2 + "/server/tls " + ignoreDir:                                             ctxTLS,
			cmdPrefix + " " + rke2 + "/server/tls/* " + ignoreDir:                                           ctxTLS,
		},
	},
	{
		distroName: "rke2_centos8",
		cmdCtx: cmdCtx{
			cmdPrefix + " " + systemD + "/rke2*":                                                       ctxUnitFile,
			cmdPrefix + " " + "/lib/systemd/system/rke2*":                                              ctxUnitFile,
			cmdPrefix + " " + "/usr/local/lib/systemd/system/rke2*":                                    ctxUnitFile,
			cmdPrefix + " " + usrBin + "/rke2":                                                         ctxExec,
			cmdPrefix + " " + usrLocal + "/rke2":                                                       ctxExec,
			cmdPrefix + " " + "/opt/cni " + ignoreDir:                                                  ctxFile,
			cmdPrefix + " " + "/opt/cni/* " + ignoreDir:                                                ctxFile,
			cmdPrefix + " " + rke2 + " " + ignoreDir:                                                   ctxVarLib,
			cmdPrefix + " " + rke2 + "/* " + ignoreDir:                                                 ctxVarLib,
			cmdPrefix + " " + rke2 + "/data " + ignoreDir:                                              ctxExec,
			cmdPrefix + " " + rke2 + "/data/* " + ignoreDir:                                            ctxExec,
			cmdPrefix + " " + rke2 + "/data/*/charts " + ignoreDir + " " + grepFilter:                  ctxConfig,
			cmdPrefix + " " + rke2 + "/data/*/charts/* " + ignoreDir + " " + grepFilter:                ctxConfig,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots " + ignoreDir + " " + grepFilter:   ctxFile,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots/* " + ignoreDir + " " + grepFilter: ctxFile,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots/*/.* " + " " + grepFilter:          ctxNone,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/sandboxes " + ignoreDir + " " + grepFilter:   ctxRoFile,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/sandboxes/* " + ignoreDir + " " + grepFilter: ctxRoFile,
			cmdPrefix + " " + rke2 + "/server/logs " + ignoreDir:                                       ctxLog,
			cmdPrefix + " " + rke2 + "/server/logs/* " + ignoreDir:                                     ctxLog,
			cmdPrefix + " " + rke2 + "/server/tls " + ignoreDir:                                        ctxTLS,
			cmdPrefix + " " + rke2 + "/server/tls/* " + ignoreDir:                                      ctxTLS,
		},
	},
	{
		distroName: "rke2_centos9",
		cmdCtx: cmdCtx{
			cmdPrefix + " " + systemD + "/rke2*":                                                          ctxUnitFile,
			cmdPrefix + " " + "/lib/systemd/system/rke2*":                                                 ctxUnitFile,
			cmdPrefix + " " + "/usr/local/lib/systemd/system/rke2*":                                       ctxUnitFile,
			cmdPrefix + " " + usrBin + "/rke2":                                                            ctxExec,
			cmdPrefix + " " + usrLocal + "/rke2":                                                          ctxExec,
			cmdPrefix + " " + "/opt/cni " + ignoreDir:                                                     ctxFile,
			cmdPrefix + " " + "/opt/cni/* " + ignoreDir:                                                   ctxFile,
			cmdPrefix + " " + rke2 + " " + ignoreDir:                                                      ctxVarLib,
			cmdPrefix + " " + rke2 + "/* " + ignoreDir:                                                    ctxVarLib,
			cmdPrefix + " " + rke2 + "/data " + ignoreDir:                                                 ctxExec,
			cmdPrefix + " " + rke2 + "/data/* " + ignoreDir:                                               ctxExec,
			cmdPrefix + " " + rke2 + "/data/*/charts " + ignoreDir + " " + grepFilter:                     ctxConfig,
			cmdPrefix + " " + rke2 + "/data/*/charts/* " + ignoreDir + " " + grepFilter:                   ctxConfig,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots " + ignoreDir + " " + grepFilter:      ctxFile,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots/ " + ignoreDir + " " + grepFilter:     ctxFile,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots/*/.* " + ignoreDir + " " + grepFilter: ctxNone,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/sandboxes " + ignoreDir + " " + grepFilter:      ctxRoFile,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/sandboxes/* " + ignoreDir + " " + grepFilter:    ctxRoFile,
			cmdPrefix + " " + rke2 + "/server/logs " + ignoreDir:                                          ctxLog,
			cmdPrefix + " " + rke2 + "/server/logs/* " + ignoreDir:                                        ctxLog,
			cmdPrefix + " " + rke2 + "/server/tls " + ignoreDir:                                           ctxTLS,
			cmdPrefix + " " + rke2 + "/server/tls/* " + ignoreDir:                                         ctxTLS,
		},
	},
	{
		// TODO: We are not able to execute this because our framework does not support the reboot part for this OS.
		distroName: "rke2_micro_os",
		cmdCtx: cmdCtx{
			cmdPrefix + " " + systemD + "/rke2*":                                                          ctxUnitFile,
			cmdPrefix + " " + "/lib/systemd/system/rke2*":                                                 ctxUnitFile,
			cmdPrefix + " " + "/usr/local/lib/systemd/system/rke2*":                                       ctxUnitFile,
			cmdPrefix + " " + usrBin + "/rke2":                                                            ctxExec,
			cmdPrefix + " " + usrLocal + "/rke2":                                                          ctxExec,
			cmdPrefix + " " + "/opt/cni " + ignoreDir:                                                     ctxFile,
			cmdPrefix + " " + "/opt/cni/* " + ignoreDir:                                                   ctxFile,
			cmdPrefix + " " + rke2 + " " + ignoreDir:                                                      ctxVarLib,
			cmdPrefix + " " + rke2 + "/* " + ignoreDir:                                                    ctxVarLib,
			cmdPrefix + " " + rke2 + "/data " + ignoreDir:                                                 ctxExec,
			cmdPrefix + " " + rke2 + "/data/* " + ignoreDir:                                               ctxExec,
			cmdPrefix + " " + rke2 + "/data/*/charts " + ignoreDir + " " + grepFilter:                     ctxConfig,
			cmdPrefix + " " + rke2 + "/data/*/charts/* " + ignoreDir + " " + grepFilter:                   ctxConfig,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots " + ignoreDir + " " + grepFilter:      ctxShare,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots/ " + ignoreDir + " " + grepFilter:     ctxShare,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots/*/.* " + ignoreDir + " " + grepFilter: ctxNone,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/sandboxes " + ignoreDir + " " + grepFilter:      ctxShare,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/sandboxes/* " + ignoreDir + " " + grepFilter:    ctxShare,
			cmdPrefix + " " + rke2 + "/server/logs " + ignoreDir:                                          ctxLog,
			cmdPrefix + " " + rke2 + "/server/logs/* " + ignoreDir:                                        ctxLog,
			cmdPrefix + " " + rke2 + "/server/tls " + ignoreDir:                                           ctxRke2TLS,
			cmdPrefix + " " + rke2 + "/server/tls/* " + ignoreDir:                                         ctxRke2TLS,
		},
	},
	{
		// TODO: We are not able to execute this because our framework does not support the reboot part for this OS.
		distroName: "rke2_sle_micro",
		cmdCtx: cmdCtx{
			cmdPrefix + " " + systemD + "/rke2*":                                                          ctxUnitFile,
			cmdPrefix + " " + "/lib/systemd/system/rke2*":                                                 ctxUnitFile,
			cmdPrefix + " " + "/usr/local/lib/systemd/system/rke2.*":                                      ctxUnitFile,
			cmdPrefix + " " + usrBin + "/rke2":                                                            ctxExec,
			cmdPrefix + " " + usrLocal + "/rke2":                                                          ctxExec,
			cmdPrefix + " " + "/opt/rke2/bin/rke2":                                                        ctxExec,
			cmdPrefix + " " + "/opt/cni " + ignoreDir:                                                     ctxFile,
			cmdPrefix + " " + "/opt/cni/* " + ignoreDir:                                                   ctxFile,
			cmdPrefix + " " + rke2 + " " + ignoreDir:                                                      ctxVarLib,
			cmdPrefix + " " + rke2 + "/* " + ignoreDir:                                                    ctxVarLib,
			cmdPrefix + " " + rke2 + "/data " + ignoreDir:                                                 ctxExec,
			cmdPrefix + " " + rke2 + "/data/*" + ignoreDir:                                                ctxExec,
			cmdPrefix + " " + rke2 + "/data/*/charts " + ignoreDir + " " + grepFilter:                     ctxConfig,
			cmdPrefix + " " + rke2 + "/data/*/charts/* " + ignoreDir + " " + grepFilter:                   ctxConfig,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots " + ignoreDir + " " + grepFilter:      ctxShare,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots/* " + ignoreDir + " " + grepFilter:    ctxShare,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots/*/.* " + ignoreDir + " " + grepFilter: ctxNone,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots/*/.* " + ignoreDir + " " + grepFilter: ctxNone,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/sandboxes " + ignoreDir + " " + grepFilter:      ctxShare,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/sandboxes/* " + ignoreDir + " " + grepFilter:    ctxShare,
			cmdPrefix + " " + rke2 + "/server/logs " + ignoreDir:                                          ctxLog,
			cmdPrefix + " " + rke2 + "/server/logs/* " + ignoreDir:                                        ctxLog,
			cmdPrefix + " " + rke2 + "/server/tls " + ignoreDir:                                           ctxTLS,
			cmdPrefix + " " + rke2 + "/server/tls/* " + ignoreDir:                                         ctxTLS,
		},
	},
	{
		// Works partially, has a bug related and some different outputs.
		distroName: "k3s_centos7",
		cmdCtx: cmdCtx{
			// TODO: issue related to UnitFile  https://github.com/k3s-io/k3s/issues/8317
			// cmdPrefix + " " + systemD + "/k3s*":                      ctxUnitFile,
			cmdPrefix + " " + "/usr/lib/systemd/system/k3s*":         ctxUnitFile,
			cmdPrefix + " " + "/usr/local/lib/systemd/system/k3s*":   ctxUnitFile,
			cmdPrefix + " " + "/usr/s?bin/k3s":                       ctxExec,
			cmdPrefix + " " + "/usr/local/s?bin/k3s":                 ctxExec,
			cmdPrefix + " " + "/var/lib/cni " + ignoreDir:            ctxVarLib,
			cmdPrefix + " " + "/var/lib/cni/* " + ignoreDir:          ctxVarLib,
			cmdPrefix + " " + "/var/lib/kubelet/pods " + ignoreDir:   ctxFile,
			cmdPrefix + " " + "/var/lib/kubelet/pods/* " + ignoreDir: ctxFile,
			/* TODO: Here the expected output is "system_u:object_r:container_var_lib_t:s0"
			and is showing this "unconfined_u:object_r:container_var_lib_t:s0" (user part is not the expected)*/
			// cmdPrefix + " " + k3s + " " + ignoreDir:                                                          ctxVarLib,
			cmdPrefix + " " + k3s + "/* " + ignoreDir:                                                        ctxVarLib,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots " + ignoreDir + " " + grepFilter:          ctxShare,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots/* " + ignoreDir + " " + grepFilter:        ctxShare,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots/*/.* " + ignoreDir + " " + grepFilter:     ctxNone,
			cmdPrefix + " " + k3s + "/agent/containerd/*/sandboxes " + ignoreDir + " " + grepFilter:          ctxShare,
			cmdPrefix + " " + k3s + "/agent/containerd/*/sandboxes/* " + ignoreDir + " " + grepFilter:        ctxShare,
			cmdPrefix + " " + k3s + "/data " + ignoreDir:                                                     ctxData,
			cmdPrefix + " " + k3s + "/data/* " + ignoreDir:                                                   ctxData,
			cmdPrefix + " " + k3s + "/data/.lock":                                                            ctxLock,
			cmdPrefix + " " + k3s + "/data/*/bin " + ignoreDir + " " + grepFilter:                            ctxRoot,
			cmdPrefix + " " + k3s + "/data/*/bin/* " + ignoreDir + " " + grepFilter:                          ctxRoot,
			cmdPrefix + " " + k3s + "/data/*/bin/.*links " + ignoreDir + " " + grepFilter:                    ctxData,
			cmdPrefix + " " + k3s + "/data/*/bin/.*sha256sums " + ignoreDir + " " + grepFilter:               ctxData,
			cmdPrefix + " " + k3s + "/data/*/bin/cni " + ignoreDir + " " + grepFilter:                        ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd " + ignoreDir + " " + grepFilter:                 ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd-shim " + ignoreDir + " " + grepFilter:            ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd-shim-runc-v[12] " + ignoreDir + " " + grepFilter: ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/runc " + ignoreDir + " " + grepFilter:                       ctxExec,
			cmdPrefix + " " + k3s + "/data/*/etc " + ignoreDir + " " + grepFilter:                            ctxConfig,
			cmdPrefix + " " + k3s + "/data/*/etc/* " + ignoreDir + " " + grepFilter:                          ctxConfig,
			cmdPrefix + " " + k3s + "/storage " + ignoreDir:                                                  ctxFile,
			cmdPrefix + " " + k3s + "/storage/* " + ignoreDir:                                                ctxFile,
			cmdPrefix + " " + "/var/log/containers " + ignoreDir:                                             ctxLog,
			cmdPrefix + " " + "/var/log/containers/* " + ignoreDir:                                           ctxLog,
			cmdPrefix + " " + "/var/log/pods " + ignoreDir:                                                   ctxLog,
			cmdPrefix + " " + "/var/log/pods/* " + ignoreDir:                                                 ctxLog,
			cmdPrefix + " " + "/var/run/flannel " + ignoreDir:                                                ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/flannel/* " + ignoreDir:                                              ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s " + ignoreDir:                                                    ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/* " + ignoreDir:                                                  ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm " + ignoreDir + " " + grepFilter:    ctxTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm/* " + ignoreDir + " " + grepFilter:  ctxTmpfs,
		},
	},
	{
		distroName: "k3s_centos8",
		cmdCtx: cmdCtx{
			// TODO: issue related to UnitFile  https://github.com/k3s-io/k3s/issues/8317
			// cmdPrefix + " " + systemD + "/k3s*":                                                          ctxUnitFile,
			cmdPrefix + " " + "/usr/lib/systemd/system/k3s*":       ctxUnitFile,
			cmdPrefix + " " + "/usr/local/lib/systemd/system/k3s*": ctxUnitFile,
			cmdPrefix + " " + "/usr/s?bin/k3s":                     ctxExec,
			cmdPrefix + " " + "/usr/local/s?bin/k3s":               ctxExec,
			/* TODO: Expected context "system_u:object_r:container_var_lib_t:s0" and is showing "unconfined_u:object_r:container_var_lib_t:s0" */
			// cmdPrefix + " " + k3s + " " + ignoreDir:                                                      ctxVarLib,
			// cmdPrefix + " " + k3s + "/* " + ignoreDir:                                                    ctxVarLib,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots " + ignoreDir + " " + grepFilter:      ctxFile,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots/* " + ignoreDir + " " + grepFilter:    ctxFile,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots/*/.* " + ignoreDir + " " + grepFilter: ctxNone,
			cmdPrefix + " " + k3s + "/agent/containerd/*/sandboxes " + ignoreDir + " " + grepFilter:      ctxRoFile,
			cmdPrefix + " " + k3s + "/agent/containerd/*/sandboxes/* " + ignoreDir + " " + grepFilter:    ctxRoFile,

			/* TODO: Expected context "system_u:object_r:k3s_data_t:s0" and is showing "unconfined_u:object_r:k3s_lock_t:s0"*/
			// cmdPrefix + " " + k3s + "/data " + ignoreDir:   ctxData,
			// cmdPrefix + " " + k3s + "/data/* " + ignoreDir: ctxData,

			/* TODO: Expected context is "system_u:object_r:k3s_lock_t:s0" and is showing "unconfined_u:object_r:k3s_lock_t:s0" */
			// cmdPrefix + " " + k3s + "/data/.lock":                                 ctxLock,

			/* TODO: For these directories output shows "unconfined_u:object_r:k3s_root_t:s0"	and the expected one is "system_u:object_r:k3s_root_t:s0"*/
			// cmdPrefix + " " + k3s + "/data/*/bin " + ignoreDir + " " + grepFilter: ctxRoot,
			// cmdPrefix + " " + k3s + "/data/*/bin/* " + ignoreDir + " " + grepFilter:                          ctxRoot,

			cmdPrefix + " " + k3s + "/data/*/bin/.*links " + ignoreDir + " " + grepFilter:                    ctxData,
			cmdPrefix + " " + k3s + "/data/*/bin/.*sha256sums " + ignoreDir + " " + grepFilter:               ctxData,
			cmdPrefix + " " + k3s + "/data/*/bin/cni " + ignoreDir + " " + grepFilter:                        ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd " + ignoreDir + " " + grepFilter:                 ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd-shim " + ignoreDir + " " + grepFilter:            ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd-shim-runc-v[12] " + ignoreDir + " " + grepFilter: ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/runc " + ignoreDir + " " + grepFilter:                       ctxExec,
			cmdPrefix + " " + k3s + "/data/*/etc " + ignoreDir + " " + grepFilter + " | grep -v 'total 0'":   ctxConfig,
			cmdPrefix + " " + k3s + "/data/*/etc/* " + ignoreDir + " " + grepFilter + " | grep -v 'total 0'": ctxConfig,
			cmdPrefix + " " + k3s + "/storage " + ignoreDir:                                                  ctxFile,
			cmdPrefix + " " + k3s + "/storage/* " + ignoreDir:                                                ctxFile,
			cmdPrefix + " " + "/var/run/k3s " + ignoreDir:                                                    ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/* " + ignoreDir:                                                  ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm " + ignoreDir + " " + grepFilter:    ctxTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm/* " + ignoreDir + " " + grepFilter:  ctxTmpfs,
		},
	},
	{
		distroName: "k3s_centos9",
		cmdCtx: cmdCtx{
			// TODO: issue related to UnitFile  https://github.com/k3s-io/k3s/issues/8317
			// cmdPrefix + " " + systemD + "/k3s*":                                                          ctxUnitFile,
			cmdPrefix + " " + "/usr/lib/systemd/system/k3s*":       ctxUnitFile,
			cmdPrefix + " " + "/usr/local/lib/systemd/system/k3s*": ctxUnitFile,
			cmdPrefix + " " + "/usr/s?bin/k3s":                     ctxExec,
			cmdPrefix + " " + "/usr/local/s?bin/k3s":               ctxExec,
			// TODO: Output: unconfined_u Expected: system_u
			// cmdPrefix + " " + k3s + " " + ignoreDir:                                                      ctxVarLib,
			// cmdPrefix + " " + k3s + "/* " + ignoreDir:                                                    ctxVarLib,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots " + ignoreDir + " " + grepFilter:      ctxFile,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots/* " + ignoreDir + " " + grepFilter:    ctxFile,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots/*/.* " + ignoreDir + " " + grepFilter: ctxNone,
			cmdPrefix + " " + k3s + "/agent/containerd/*/sandboxes " + ignoreDir + " " + grepFilter:      ctxRoFile,
			cmdPrefix + " " + k3s + "/agent/containerd/*/sandboxes/* " + ignoreDir + " " + grepFilter:    ctxRoFile,
			/* TODO: Expected "system_u:object_r:k3s_data_t:s0" and is showing "unconfined_u:object_r:k3s_data_t:s0" */
			// cmdPrefix + " " + k3s + "/data " + ignoreDir:                                                 ctxData,
			// cmdPrefix + " " + k3s + "/data/* " + ignoreDir:                                               ctxData,

			/* TODO: Expected "system_u:object_r:k3s_lock_t:s0 " and is showing "unconfined_u:object_r:k3s_lock_t:s0"*/
			// cmdPrefix + " " + k3s + "/data/.lock":                                                            ctxLock,

			/* TODO: Expected "system_u:object_r:k3s_root_t:s0 " and is showing "unconfined_u:object_r:k3s_root_t:s0" */
			// cmdPrefix + " " + k3s + "/data/*/bin " + ignoreDir + " " + grepFilter:                            ctxRoot,
			// cmdPrefix + " " + k3s + "/data/*/bin/* " + ignoreDir + " " + grepFilter:                          ctxRoot,

			cmdPrefix + " " + k3s + "/data/*/bin/.*links " + ignoreDir + " " + grepFilter:                    ctxData,
			cmdPrefix + " " + k3s + "/data/*/bin/.*sha256sums " + ignoreDir + " " + grepFilter:               ctxData,
			cmdPrefix + " " + k3s + "/data/*/bin/cni " + ignoreDir + " " + grepFilter:                        ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd " + ignoreDir + " " + grepFilter:                 ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd-shim " + ignoreDir + " " + grepFilter:            ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd-shim-runc-v[12] " + ignoreDir + " " + grepFilter: ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/runc " + ignoreDir + " " + grepFilter:                       ctxExec,
			cmdPrefix + " " + k3s + "/data/*/etc " + ignoreDir + " " + grepFilter + " | grep -v 'total 0'":   ctxConfig,
			cmdPrefix + " " + k3s + "/data/*/etc/* " + ignoreDir + " " + grepFilter + " | grep -v 'total 0'": ctxConfig,
			cmdPrefix + " " + k3s + "/storage " + ignoreDir:                                                  ctxFile,
			cmdPrefix + " " + k3s + "/storage/* " + ignoreDir:                                                ctxFile,
			cmdPrefix + " " + "/var/run/k3s " + ignoreDir:                                                    ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/* " + ignoreDir:                                                  ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm " + ignoreDir + " " + grepFilter:    ctxTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm/* " + ignoreDir + " " + grepFilter:  ctxTmpfs,
		},
	},
	{
		// TODO: We are not able to execute this because our framework does not support the reboot part for this OS.
		distroName: "k3s_coreos",
		cmdCtx: cmdCtx{
			cmdPrefix + " " + systemD + "/k3s*":                                                              ctxUnitFile,
			cmdPrefix + " " + "/usr/lib/systemd/system/k3s*":                                                 ctxUnitFile,
			cmdPrefix + " " + "/usr/local/lib/systemd/system/k3s*":                                           ctxUnitFile,
			cmdPrefix + " " + "/usr/s?bin/k3s":                                                               ctxExec,
			cmdPrefix + " " + "/usr/local/s?bin/k3s":                                                         ctxExec,
			cmdPrefix + " " + k3s + " " + ignoreDir:                                                          ctxVarLib,
			cmdPrefix + " " + k3s + "/* " + ignoreDir:                                                        ctxVarLib,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots " + ignoreDir + " " + grepFilter:          ctxFile,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots/* " + ignoreDir + " " + grepFilter:        ctxFile,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots/*/.* " + ignoreDir + " " + grepFilter:     ctxNone,
			cmdPrefix + " " + k3s + "/agent/containerd/*/sandboxes " + ignoreDir + " " + grepFilter:          ctxShare,
			cmdPrefix + " " + k3s + "/agent/containerd/*/sandboxes/* " + ignoreDir + " " + grepFilter:        ctxShare,
			cmdPrefix + " " + k3s + "/data " + ignoreDir:                                                     ctxData,
			cmdPrefix + " " + k3s + "/data/* " + ignoreDir:                                                   ctxData,
			cmdPrefix + " " + k3s + "/data/.lock":                                                            ctxLock,
			cmdPrefix + " " + k3s + "/data/*/bin " + ignoreDir + " " + grepFilter:                            ctxRoot,
			cmdPrefix + " " + k3s + "/data/*/bin/* " + ignoreDir + " " + grepFilter:                          ctxRoot,
			cmdPrefix + " " + k3s + "/data/*/bin/.*links " + ignoreDir + " " + grepFilter:                    ctxData,
			cmdPrefix + " " + k3s + "/data/*/bin/.*sha256sums " + ignoreDir + " " + grepFilter:               ctxData,
			cmdPrefix + " " + k3s + "/data/*/bin/cni " + ignoreDir + " " + grepFilter:                        ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd " + ignoreDir + " " + grepFilter:                 ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd-shim " + ignoreDir + " " + grepFilter:            ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd-shim-runc-v[12] " + ignoreDir + " " + grepFilter: ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/runc " + ignoreDir + " " + grepFilter:                       ctxExec,
			cmdPrefix + " " + k3s + "/data/*/etc " + ignoreDir + " " + grepFilter:                            ctxConfig,
			cmdPrefix + " " + k3s + "/data/*/etc/* " + ignoreDir + " " + grepFilter:                          ctxConfig,
			cmdPrefix + " " + k3s + "/storage " + ignoreDir:                                                  ctxFile,
			cmdPrefix + " " + k3s + "/storage/* " + ignoreDir:                                                ctxFile,
			cmdPrefix + " " + "/var/run/k3s " + ignoreDir:                                                    ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/* " + ignoreDir:                                                  ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm " + ignoreDir + " " + grepFilter:    ctxTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm/* " + ignoreDir + " " + grepFilter:  ctxTmpfs,
		},
	},
	{
		// TODO: We are not able to execute this because our framework does not support the reboot part for this OS.
		distroName: "k3s_micro_os",
		cmdCtx: cmdCtx{
			cmdPrefix + " " + systemD + "/k3s*":                                                              ctxUnitFile,
			cmdPrefix + " " + "/usr/lib/systemd/system/k3s*":                                                 ctxUnitFile,
			cmdPrefix + " " + "/usr/local/lib/systemd/system/k3s*":                                           ctxUnitFile,
			cmdPrefix + " " + "/usr/s?bin/k3s":                                                               ctxExec,
			cmdPrefix + " " + "/usr/local/s?bin/k3s":                                                         ctxExec,
			cmdPrefix + " " + k3s + " " + ignoreDir:                                                          ctxVarLib,
			cmdPrefix + " " + k3s + "/* " + ignoreDir:                                                        ctxVarLib,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots " + ignoreDir + " " + grepFilter:          ctxFile,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots/* " + ignoreDir + " " + grepFilter:        ctxFile,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots/*/.* " + ignoreDir + " " + grepFilter:     ctxNone,
			cmdPrefix + " " + k3s + "/agent/containerd/*/sandboxes " + ignoreDir + " " + grepFilter:          ctxShare,
			cmdPrefix + " " + k3s + "/agent/containerd/*/sandboxes/* " + ignoreDir + " " + grepFilter:        ctxShare,
			cmdPrefix + " " + k3s + "/data " + ignoreDir:                                                     ctxData,
			cmdPrefix + " " + k3s + "/data/* " + ignoreDir:                                                   ctxData,
			cmdPrefix + " " + k3s + "/data/.lock":                                                            ctxLock,
			cmdPrefix + " " + k3s + "/data/*/bin " + ignoreDir + " " + grepFilter:                            ctxRoot,
			cmdPrefix + " " + k3s + "/data/*/bin/* " + ignoreDir + " " + grepFilter:                          ctxRoot,
			cmdPrefix + " " + k3s + "/data/*/bin/.*links " + ignoreDir + " " + grepFilter:                    ctxData,
			cmdPrefix + " " + k3s + "/data/*/bin/.*sha256sums " + ignoreDir + " " + grepFilter:               ctxData,
			cmdPrefix + " " + k3s + "/data/*/bin/cni " + ignoreDir + " " + grepFilter:                        ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd " + ignoreDir + " " + grepFilter:                 ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd-shim " + ignoreDir + " " + grepFilter:            ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd-shim-runc-v[12] " + ignoreDir + " " + grepFilter: ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/runc " + ignoreDir + " " + grepFilter:                       ctxExec,
			cmdPrefix + " " + k3s + "/data/*/etc " + ignoreDir + " " + grepFilter:                            ctxConfig,
			cmdPrefix + " " + k3s + "/data/*/etc/* " + ignoreDir + " " + grepFilter:                          ctxConfig,
			cmdPrefix + " " + k3s + "/storage " + ignoreDir:                                                  ctxFile,
			cmdPrefix + " " + k3s + "/storage/* " + ignoreDir:                                                ctxFile,
			cmdPrefix + " " + "/var/run/k3s " + ignoreDir:                                                    ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/* " + ignoreDir:                                                  ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm " + ignoreDir + " " + grepFilter:    ctxTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm/* " + ignoreDir + " " + grepFilter:  ctxTmpfs,
		},
	},
	{
		// TODO: We are not able to execute this because our framework does not support the reboot part for this OS.
		distroName: "k3s_sle_micro",
		cmdCtx: cmdCtx{
			cmdPrefix + " " + systemD + "/k3s*":                                                              ctxUnitFile,
			cmdPrefix + " " + "/usr/lib/systemd/system/k3s*":                                                 ctxUnitFile,
			cmdPrefix + " " + "/usr/local/lib/systemd/system/k3s*":                                           ctxUnitFile,
			cmdPrefix + " " + "/usr/s?bin/k3s":                                                               ctxExec,
			cmdPrefix + " " + "/usr/local/s?bin/k3s":                                                         ctxExec,
			cmdPrefix + " " + k3s + " " + ignoreDir:                                                          ctxVarLib,
			cmdPrefix + " " + k3s + "/* " + ignoreDir:                                                        ctxVarLib,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots " + ignoreDir + " " + grepFilter:          ctxShare,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots/* " + ignoreDir + " " + grepFilter:        ctxShare,
			cmdPrefix + " " + k3s + "/agent/containerd/*/snapshots/*/.* " + ignoreDir + " " + grepFilter:     ctxNone,
			cmdPrefix + " " + k3s + "/agent/containerd/*/sandboxes " + ignoreDir + " " + grepFilter:          ctxShare,
			cmdPrefix + " " + k3s + "/agent/containerd/*/sandboxes/* " + ignoreDir + " " + grepFilter:        ctxShare,
			cmdPrefix + " " + k3s + "/data " + ignoreDir:                                                     ctxData,
			cmdPrefix + " " + k3s + "/data/* " + ignoreDir:                                                   ctxData,
			cmdPrefix + " " + k3s + "/data/.lock":                                                            ctxLock,
			cmdPrefix + " " + k3s + "/data/*/bin " + ignoreDir + " " + grepFilter:                            ctxRoot,
			cmdPrefix + " " + k3s + "/data/*/bin/* " + ignoreDir + " " + grepFilter:                          ctxRoot,
			cmdPrefix + " " + k3s + "/data/*/bin/.*links " + ignoreDir + " " + grepFilter:                    ctxData,
			cmdPrefix + " " + k3s + "/data/*/bin/.*sha256sums " + ignoreDir + " " + grepFilter:               ctxData,
			cmdPrefix + " " + k3s + "/data/*/bin/cni " + ignoreDir + " " + grepFilter:                        ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd " + ignoreDir + " " + grepFilter:                 ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd-shim " + ignoreDir + " " + grepFilter:            ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/containerd-shim-runc-v[12] " + ignoreDir + " " + grepFilter: ctxExec,
			cmdPrefix + " " + k3s + "/data/*/bin/runc " + ignoreDir + " " + grepFilter:                       ctxExec,
			cmdPrefix + " " + k3s + "/data/*/etc " + ignoreDir + " " + grepFilter:                            ctxConfig,
			cmdPrefix + " " + k3s + "/data/*/etc/* " + ignoreDir + " " + grepFilter:                          ctxConfig,
			cmdPrefix + " " + k3s + "/storage " + ignoreDir:                                                  ctxFile,
			cmdPrefix + " " + k3s + "/storage/* " + ignoreDir:                                                ctxFile,
			cmdPrefix + " " + "/var/run/k3s " + ignoreDir:                                                    ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/* " + ignoreDir:                                                  ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm " + ignoreDir + " " + grepFilter:    ctxTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm/* " + ignoreDir + " " + grepFilter:  ctxTmpfs,
		},
	},
}
