package testcase

import (
	"fmt"
	"log"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/shared"
)

// TestSelinuxEnabled Validates that containerd is running with selinux enabled in the config
func TestSelinuxEnabled() {
	product, err := shared.GetProduct()
	if err != nil {
		return
	}

	ips := shared.FetchNodeExternalIP()
	selinuxConfigAssert := "selinux: true"
	selinuxContainerdAssert := "enable_selinux = true"

	for _, ip := range ips {
		err := assert.CheckComponentCmdNode("cat /etc/rancher/"+
			product+"/config.yaml", ip, selinuxConfigAssert)
		Expect(err).NotTo(HaveOccurred())
		errCont := assert.CheckComponentCmdNode("sudo cat /var/lib/rancher/"+
			product+"/agent/etc/containerd/config.toml", ip, selinuxContainerdAssert)
		Expect(errCont).NotTo(HaveOccurred())
	}
}

// TestSelinuxVersions Validates container-selinux version, rke2-selinux version and rke2-selinux version
func TestSelinuxVersions() {
	cluster := factory.AddCluster(GinkgoT())
	product, err := shared.GetProduct()
	if err != nil {
		return
	}

	var serverCmd string
	var serverAsserts []string
	agentAsserts := []string{"container-selinux", product + "-selinux"}

	switch product {
	case "k3s":
		serverCmd = "rpm -qa container-selinux k3s-selinux"
		serverAsserts = []string{"container-selinux", "k3s-selinux"}
	default:
		serverCmd = "rpm -qa container-selinux rke2-server rke2-selinux"
		serverAsserts = []string{"container-selinux", "rke2-selinux", "rke2-server"}
	}

	if cluster.NumServers > 0 {
		for _, serverIP := range cluster.ServerIPs {
			err := assert.CheckComponentCmdNode(serverCmd, serverIP, serverAsserts...)
			Expect(err).NotTo(HaveOccurred())
		}
	}

	if cluster.NumAgents > 0 {
		for _, agentIP := range cluster.AgentIPs {
			err := assert.CheckComponentCmdNode("rpm -qa container-selinux "+product+"-selinux", agentIP, agentAsserts...)
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

// https://github.com/k3s-io/k3s/blob/master/install.sh
// https://github.com/rancher/rke2/blob/master/install.sh
// Based on this info, this is the way to validate the correct context
func getContext(product string, ip string) map[string]string {
	rke2_centos7 := map[string]string{
		// https://github.com/rancher/rke2-selinux/blob/master/policy/centos7/rke2.fc
		"sudo ls -laZ /etc/systemd/system/rke2*":                                      "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /lib/systemd/system/rke2*":                                      "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/local/lib/systemd/system/rke2*":                            "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/bin/rke2":                                                  "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /usr/local/bin/rke2":                                            "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/cni -I .. -I .":                                        "system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /var/lib/cni/* -I .. -I .":                                      "system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /opt/cni -I .. -I .":                                            "system_u:object_r:container_file_t",
		"sudo ls -laZ /opt/cni/* -I . -I ..":                                          "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/kubelet/pods -I .. -I .":                               "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/kubelet/pods/* -I .. -I .":                             "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/rke2 -I .. -I .":                               "system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /var/lib/rancher/rke2/* -I .. -I .":                             "system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /var/lib/rancher/rke2/data(/.*)?":                               "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/rke2/data/*/charts -I .. -I .":                 "system_u:object_r:container_config_t",
		"sudo ls -laZ /var/lib/rancher/rke2/data/*/charts/*":                          "system_u:object_r:container_config_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/*/snapshots -I .. -I .":  "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/*/snapshots/ -I .. -I .": "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/*/snapshots/[^/]*/.*":    "",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/*/sandboxes -I .. -I .":  "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/*/sandboxes/ -I .. -I .": "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/rke2/server/logs -I .. -I .":                   "system_u:object_r:container_log_t",
		"sudo ls -laZ /var/lib/rancher/rke2/server/logs/ -I .. -I .":                  "system_u:object_r:container_log_t",
		"sudo ls -laZ /var/run/flannel -I .. -I .":                                    "system_u:object_r:container_var_run_t",
		"sudo ls -laZ /var/run/flannel/* -I .. -I .":                                  "system_u:object_r:container_var_run_t",
		"sudo ls -laZ /var/run/k3s -I .. -I .":                                        "system_u:object_r:container_var_run_t",
		"sudo ls -laZ /var/run/k3s/* -I .. -I .":                                      "system_u:object_r:container_var_run_t",
		"sudo ls -laZ /var/run/k3s/containerd/*/sandboxes/*/shm":                      "system_u:object_r:container_runtime_tmpfs_t",
		"sudo ls -laZ /var/run/k3s/containerd/*/sandboxes/*/shm/*":                    "system_u:object_r:container_runtime_tmpfs_t",
		"sudo ls -laZ /var/log/containers -I .. -I .":                                 "system_u:object_r:container_log_t",
		"sudo ls -laZ /var/log/containers/* -I .. -I .":                               "system_u:object_r:container_log_t",
		"sudo ls -laZ /var/log/pods -I .. -I .":                                       "system_u:object_r:container_log_t",
		"sudo ls -laZ /var/log/pods/* -I .. -I .":                                     "system_u:object_r:container_log_t",
		"sudo ls -laZ /var/lib/rancher/rke2/server/tls -I .. -I .":                    "system_u:object_r:rke2_tls_t",
		"sudo ls -laZ /var/lib/rancher/rke2/server/tls/* -I .. -I .":                  "system_u:object_r:rke2_tls_t",
	}
	rke2_centos8 := map[string]string{
		// https://github.com/rancher/rke2-selinux/blob/master/policy/centos8/rke2.fc
		"sudo ls -laZ /etc/systemd/system/rke2*":                                      "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /lib/systemd/system/rke2*":                                      "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/local/lib/systemd/system/rke2*":                            "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/bin/rke2":                                                  "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /usr/local/bin/rke2":                                            "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /opt/cni -I .. -I .":                                            "system_u:object_r:container_file_t",
		"sudo ls -laZ /opt/cni/* -I .. -I .":                                          "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/rke2(/.*)?":                                    "system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /var/lib/rancher/rke2/data(/.*)?":                               "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/rke2/data/[^/]/charts(/.*)?":                   "system_u:object_r:container_config_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/[^/]*/snapshots":         "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/[^/]*/snapshots/[^/]*":   "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/[^/]*/snapshots/[^/]/.*": "<<none>>",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/[^/]*/sandboxes(/.*)?":   "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/rke2/server/logs(/.*)?":                        "system_u:object_r:container_log_t",
		"sudo ls -laZ /var/lib/rancher/rke2/server/tls(/.*)?":                         "system_u:object_r:rke2_tls_t",
	}
	rke2_centos9 := map[string]string{
		// https://github.com/rancher/rke2-selinux/blob/master/policy/centos9/rke2.fc
		"sudo ls -laZ /etc/systemd/system/rke2*":                                       "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /lib/systemd/system/rke2*":                                       "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/local/lib/systemd/system/rke2*":                             "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/bin/rke2":                                                   "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /usr/local/bin/rke2":                                             "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /opt/cni -I .. -I .":                                             "system_u:object_r:container_file_t",
		"sudo ls -laZ /opt/cni/* -I . -I ..":                                           "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/rke2 -I .. -I .":                                "system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /var/lib/rancher/rke2/* -I .. -I .":                              "system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /var/lib/rancher/rke2/data -I .. -I .":                           "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/rke2/data/* -I .. -I .":                         "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/rke2/data/*/charts -I .. -I .":                  "system_u:object_r:container_config_t",
		"sudo ls -laZ /var/lib/rancher/rke2/data/*/charts/*":                           "system_u:object_r:container_config_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/*/snapshots -I . -I ..":   "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/*/snapshots/ -I . -I ..":  "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/[^/]*/snapshots/[^/]*/.*": "",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/*/sandboxes -I .. -I .":   "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/*/sandboxes/ -I .. -I .":  "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/rke2/server/logs -I .. -I .":                    "system_u:object_r:container_log_t",
		"sudo ls -laZ /var/lib/rancher/rke2/server/tls -I .. -I .":                     "system_u:object_r:rke2_tls_t",
	}
	rke2_micro_os := map[string]string{
		// https://github.com/rancher/rke2-selinux/blob/master/policy/microos/rke2.fc
		"sudo ls -laZ /etc/systemd/system/rke2*":                                      "	system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /lib/systemd/system/rke2*":                                      "	system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/local/lib/systemd/system/rke2.*":                           "	system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/bin/rke2":                                                  "	system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /usr/local/bin/rke2":                                            "	system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /opt/cni(/.*)?":                                                 "	system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/rke2(/.*)?":                                    "	system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /var/lib/rancher/rke2/data(/.*)?":                               "	system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/rke2/data/[^/]/charts(/.*)?":                   "	system_u:object_r:container_config_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/[^/]*/snapshots":         "	system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/[^/]*/snapshots/[^/]":    "	system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/[^/]*/snapshots/[^/]/.*": "	<<none>>",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/[^/]*/sandboxes(/.)?":    "	system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/rke2/server/logs(/.*)?":                        "	system_u:object_r:container_log_t",
		"sudo ls -laZ /var/lib/rancher/rke2/server/tls(/.*)?":                         "	system_u:object_r:rke2_tls_t",
	}
	rke2_sle_micro := map[string]string{
		// https://github.com/rancher/rke2-selinux/blob/master/policy/slemicro/rke2.fc
		"sudo ls -laZ /etc/systemd/system/rke2*":                                      "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /lib/systemd/system/rke2*":                                      "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/local/lib/systemd/system/rke2.*":                           "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/bin/rke2	":                                                 "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /usr/local/bin/rke2":                                            "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /opt/rke2/bin/rke2":                                             "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /opt/cni(/.*)?":                                                 "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/rke2(/.*)?":                                    "system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /var/lib/rancher/rke2/data(/.*)?":                               "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/rke2/data/[^/]/charts(/.*)?":                   "system_u:object_r:container_config_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/[^/]*/snapshots":         "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/[^/]*/snapshots/[^/]":    "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/[^/]*/snapshots/[^/]/.*": "<<none>>",
		"sudo ls -laZ /var/lib/rancher/rke2/agent/containerd/[^/]*/sandboxes(/.)?":    "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/rke2/server/logs(/.*)?":                        "system_u:object_r:container_log_t",
		"sudo ls -laZ /var/lib/rancher/rke2/server/tls(/.*)?":                         "system_u:object_r:rke2_tls_t",
	}
	k3s_centos7 := map[string]string{
		"sudo ls -laZ /etc/systemd/system/k3s*":                                       "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/lib/systemd/system/k3s*":                                   "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/local/lib/systemd/system/k3s*":                             "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/s?bin/k3s":                                                 "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /usr/local/s?bin/k3s":                                           "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/cni(/.*)?":                                             "system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /var/lib/kubelet/pods(/.*)?":                                    "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/k3s(/.*)?":                                     "system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots":          "-d system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots/[^/]*":    "-d system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots/[^/]*/.*": "<<none>>",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/sandboxes(/.*)?":    "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data(/.*)?":                                "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/.lock":                                "system_u:object_r:k3s_lock_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin(/.*)?":                      "system_u:object_r:k3s_root_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/[.]links":                   "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/[.]sha256sums":              "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/cni":                        "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd":                 "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd-shim":            "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd-shim-runc-v[12]": "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/runc":                       "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/etc(/.*)?":                      "system_u:object_r:container_config_t",
		"sudo ls -laZ /var/lib/rancher/k3s/storage(/.*)?":                             "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/log/containers(/.*)?":                                      "system_u:object_r:container_log_t",
		"sudo ls -laZ /var/log/pods(/.*)?":                                            "system_u:object_r:container_log_t",
		"sudo ls -laZ /var/run/flannel(/.*)?":                                         "system_u:object_r:container_var_run_t",
		"sudo ls -laZ /var/run/k3s(/.*)?":                                             "system_u:object_r:container_var_run_t",
		"sudo ls -laZ /var/run/k3s/containerd/[^/]*/sandboxes/[^/]*/shm(/.*)?":        "system_u:object_r:container_runtime_tmpfs_t",
	}
	k3s_centos8 := map[string]string{
		"sudo ls -laZ /etc/systemd/system/k3s*":                                       "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/lib/systemd/system/k3s*":                                   "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/local/lib/systemd/system/k3s*":                             "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/local/bin/k3s":                                             "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s(/.*)?":                                     "system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots":          "-d system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots/[^/]*":    "-d system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots/[^/]*/.*": "<<none>>",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/sandboxes(/.*)?":    "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data(/.*)?":                                "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/.lock":                                "system_u:object_r:k3s_lock_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin(/.*)?":                      "system_u:object_r:k3s_root_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/[.]links":                   "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/[.]sha256sums":              "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/cni":                        "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd":                 "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd-shim":            "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd-shim-runc-v[12]": "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/runc":                       "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/etc(/.*)?":                      "system_u:object_r:container_config_t",
		"sudo ls -laZ /var/lib/rancher/k3s/storage(/.*)?":                             "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/run/k3s(/.*)?":                                             "system_u:object_r:container_var_run_t",
		"sudo ls -laZ /var/run/k3s/containerd/[^/]*/sandboxes/[^/]*/shm(/.*)?":        "system_u:object_r:container_runtime_tmpfs_t",
	}
	k3s_centos9 := map[string]string{
		"sudo ls -laZ /etc/systemd/system/k3s*":                                       "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/lib/systemd/system/k3s*":                                   "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/local/lib/systemd/system/k3s*":                             "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/s?bin/k3s":                                                 "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /usr/local/s?bin/k3s":                                           "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s(/.*)?":                                     "system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots":          "-d system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots/[^/]*":    "-d system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots/[^/]*/.*": "<<none>>",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/sandboxes(/.*)?":    "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data(/.*)?":                                "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/.lock":                                "system_u:object_r:k3s_lock_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin(/.*)?":                      "system_u:object_r:k3s_root_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/[.]links":                   "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/[.]sha256sums":              "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/cni":                        "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd":                 "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd-shim":            "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd-shim-runc-v[12]": "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/runc":                       "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/etc(/.*)?":                      "system_u:object_r:container_config_t",
		"sudo ls -laZ /var/lib/rancher/k3s/storage(/.*)?":                             "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/run/k3s(/.*)?":                                             "system_u:object_r:container_var_run_t",
		"sudo ls -laZ /var/run/k3s/containerd/[^/]*/sandboxes/[^/]*/shm(/.*)?":        "system_u:object_r:container_runtime_tmpfs_t",
	}
	k3s_coreos := map[string]string{
		"sudo ls -laZ /etc/systemd/system/k3s*":                                       "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/lib/systemd/system/k3s*":                                   "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/local/lib/systemd/system/k3s*":                             "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/s?bin/k3s":                                                 "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /usr/local/s?bin/k3s":                                           "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s(/.*)?":                                     "system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots":          "-d system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots/[^/]*":    "-d system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots/[^/]*/.*": "<<none>>",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/sandboxes(/.*)?":    "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data(/.*)?":                                "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/.lock":                                "system_u:object_r:k3s_lock_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin(/.*)?":                      "system_u:object_r:k3s_root_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/[.]links":                   "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/[.]sha256sums":              "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/cni":                        "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd":                 "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd-shim":            "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd-shim-runc-v[12]": "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/runc":                       "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/etc(/.*)?":                      "system_u:object_r:container_config_t",
		"sudo ls -laZ /var/lib/rancher/k3s/storage(/.*)?":                             "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/run/k3s(/.*)?":                                             "system_u:object_r:container_var_run_t",
		"sudo ls -laZ /var/run/k3s/containerd/[^/]*/sandboxes/[^/]*/shm(/.*)?":        "system_u:object_r:container_runtime_tmpfs_t",
	}
	k3s_micro_os := map[string]string{
		"sudo ls -laZ /etc/systemd/system/k3s*":                                       "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/lib/systemd/system/k3s*":                                   "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/local/lib/systemd/system/k3s*":                             "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/s?bin/k3s":                                                 "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /usr/local/s?bin/k3s":                                           "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s(/.*)?":                                     "system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots":          "-d system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots/[^/]*":    "-d system_u:object_r:container_file_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots/[^/]*/.*": "<<none>>",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/sandboxes(/.*)?":    "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data(/.*)?":                                "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/.lock":                                "system_u:object_r:k3s_lock_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin(/.*)?":                      "system_u:object_r:k3s_root_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/[.]links":                   "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/[.]sha256sums":              "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/cni":                        "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd":                 "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd-shim":            "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd-shim-runc-v[12]": "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/runc":                       "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/etc(/.*)?":                      "system_u:object_r:container_config_t",
		"sudo ls -laZ /var/lib/rancher/k3s/storage(/.*)?":                             "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/run/k3s(/.*)?":                                             "system_u:object_r:container_var_run_t",
		"sudo ls -laZ /var/run/k3s/containerd/[^/]*/sandboxes/[^/]*/shm(/.*)?":        "system_u:object_r:container_runtime_tmpfs_t",
	}
	k3s_sle_micro := map[string]string{
		"sudo ls -laZ /etc/systemd/system/k3s*":                                       "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/lib/systemd/system/k3s*":                                   "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/local/lib/systemd/system/k3s*":                             "system_u:object_r:container_unit_file_t",
		"sudo ls -laZ /usr/s?bin/k3s":                                                 "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /usr/local/s?bin/k3s":                                           "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s(/.*)?":                                     "system_u:object_r:container_var_lib_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots":          "-d system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots/[^/]*":    "-d system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/snapshots/[^/]*/.*": "<<none>>",
		"sudo ls -laZ /var/lib/rancher/k3s/agent/containerd/[^/]*/sandboxes(/.*)?":    "system_u:object_r:container_share_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data(/.*)?":                                "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/.lock":                                "system_u:object_r:k3s_lock_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin(/.*)?":                      "system_u:object_r:k3s_root_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/[.]links":                   "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/[.]sha256sums":              "system_u:object_r:k3s_data_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/cni":                        "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd":                 "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd-shim":            "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/containerd-shim-runc-v[12]": "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/bin/runc":                       "system_u:object_r:container_runtime_exec_t",
		"sudo ls -laZ /var/lib/rancher/k3s/data/[^/]*/etc(/.*)?":                      "system_u:object_r:container_config_t",
		"sudo ls -laZ /var/lib/rancher/k3s/storage(/.*)?":                             "system_u:object_r:container_file_t",
		"sudo ls -laZ /var/run/k3s(/.*)?":                                             "system_u:object_r:container_var_run_t",
		"sudo ls -laZ /var/run/k3s/containerd/[^/]*/sandboxes/[^/]*/shm(/.*)?":        "system_u:object_r:container_runtime_tmpfs_t",
	}

	res, err := shared.RunCommandOnNode("cat /etc/os-release", ip)
	Expect(err).NotTo(HaveOccurred())

	if strings.Contains(res, "ID_LIKE='suse'") {
		if strings.Contains(res, "VARIANT_ID='sle-micro'") {
			if product == "k3s" {
				fmt.Println("Using 'slemicro' policy for this K3S cluster.")
				return k3s_sle_micro
			} else {
				fmt.Println("Using 'slemicro' policy for this RKE2 cluster.")
				return rke2_sle_micro
			}
		}
		if product == "k3s" {
			fmt.Println("Using 'microos' policy for this K3S cluster")
			return k3s_micro_os
		} else {
			fmt.Println("Using 'microos' policy for this RKE2 cluster.")
			return rke2_micro_os
		}
	}
	if strings.Contains(res, "ID_LIKE='coreos'") || strings.Contains(res, "VARIANT_ID='coreos'") {
		fmt.Println("Using 'coreos' policy for this k3s cluster")
		return k3s_coreos
	}
	if strings.Contains(res, "VERSION_ID") {
		res, err := shared.RunCommandOnNode("cat /etc/os-release | grep 'VERSION_ID'", ip)
		Expect(err).NotTo(HaveOccurred())

		parts := strings.Split(res, "=")

		if len(parts) == 2 {
			version := strings.Trim(parts[1], "\"")
			if strings.HasPrefix(version, "7") {
				if product == "k3s" {
					fmt.Println("Using 'centos7' policy for this K3S cluster")
					return k3s_centos7
				} else {
					fmt.Println("Using 'centos7' policy for this RKE2 cluster.")
					return rke2_centos7
				}
			}
			if strings.HasPrefix(version, "8") {
				if product == "k3s" {
					fmt.Println("Using 'centos8' policy for this K3S cluster")
					return k3s_centos8
				} else {
					fmt.Println("Using 'centos8' policy for this RKE2 cluster")
					return rke2_centos8
				}
			}
			if strings.HasPrefix(version, "9") {
				if product == "k3s" {
					fmt.Println("Using 'centos9' policy for this K3S cluster")
					return k3s_centos9
				} else {
					fmt.Println("Using 'centos9' policy for this RKE2 cluster")
					return rke2_centos9
				}
			}
		}
	}

	return rke2_micro_os
}

// TestSelinuxContext Validates directories to ensure they have the correct selinux contexts created
func TestSelinuxContext() {
	cluster := factory.AddCluster(GinkgoT())
	product, err := shared.GetProduct()
	if err != nil {
		log.Println(err)
	}

	if cluster.NumServers > 0 {
		for _, ip := range cluster.ServerIPs {

			var context map[string]string

			context = getContext(product, ip)

			for cmd, expectedContext := range context {
				res, err := shared.RunCommandOnNode(cmd, ip)
				fmt.Println("\nResult from run cmd: ", cmd, " || Expected result: ", expectedContext)
				fmt.Println("Result: ", res)
				if res != "" {
					Expect(res).Should(ContainSubstring(expectedContext), "Error on cmd %v \n Context %v \nnot found on ", cmd, expectedContext, res)
					Expect(err).NotTo(HaveOccurred())
				}
				if strings.Contains(res, "No such file or directory") {
					fmt.Println("No such file or directory !!", err)
				}
				fmt.Println(err)
			}
		}
	}
}

// TestSelinuxSpcT Validate that containers don't run with spc_t
func TestSelinuxSpcT() {
	cluster := factory.AddCluster(GinkgoT())

	for _, serverIP := range cluster.ServerIPs {
		res, err := shared.RunCommandOnNode("ps auxZ | grep metrics | grep -v grep", serverIP)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).ShouldNot(ContainSubstring("spc_t"))
	}
}

// TestUninstallPolicy Validate that un-installation will remove the rke2-selinux or k3s-selinux policy
func TestUninstallPolicy() {
	product, err := shared.GetProduct()
	if err != nil {
		log.Println(err)
	}
	cluster := factory.AddCluster(GinkgoT())
	var serverUninstallCmd string
	var agentUninstallCmd string

	switch product {
	case "k3s":
		serverUninstallCmd = "k3s-uninstall.sh"
		agentUninstallCmd = "k3s-agent-uninstall.sh"

	default:
		serverUninstallCmd = "sudo rke2-uninstall.sh"
		agentUninstallCmd = "sudo rke2-uninstall.sh"
	}

	for _, serverIP := range cluster.ServerIPs {
		fmt.Println("Uninstalling "+product+" on server: ", serverIP)

		_, err := shared.RunCommandOnNode(serverUninstallCmd, serverIP)
		Expect(err).NotTo(HaveOccurred())

		res, errSel := shared.RunCommandOnNode("rpm -qa container-selinux "+product+"-server "+product+"-selinux", serverIP)
		Expect(errSel).NotTo(HaveOccurred())
		Expect(res).Should(BeEmpty())
	}

	for _, agentIP := range cluster.AgentIPs {
		fmt.Println("Uninstalling "+product+" on agent: ", agentIP)

		_, err := shared.RunCommandOnNode(agentUninstallCmd, agentIP)
		Expect(err).NotTo(HaveOccurred())

		res, errSel := shared.RunCommandOnNode("rpm -qa container-selinux "+product+"-selinux", agentIP)
		Expect(errSel).NotTo(HaveOccurred())
		Expect(res).Should(BeEmpty())
	}
}
