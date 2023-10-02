package testcase

var (
	cmdPrefix = "sudo ls -laZ"
	ignoreDir = "-I .. -I ."
	rke2      = "/var/lib/rancher/rke2"
	systemD   = "/etc/systemd/system"
	usrBin    = "/usr/bin"
	usrLocal  = "/usr/local/bin"
)

const (
	ctxUnitFile = "system_u:object_r:container_unit_file_t:s0"
	ctxExec     = "system_u:object_r:container_runtime_exec_t:s0"
	ctxVarLib   = "system_u:object_r:container_var_lib_t:s0"
	ctxFile     = "system_u:object_r:container_file_t:s0"
	ctxConfig   = "system_u:object_r:container_config_t:s0"
	ctxShare    = "system_u:object_r:container_share_t:s0"
	ctxLog      = "system_u:object_r:container_log_t:s0"
	ctxRunTmpfs = "system_u:object_r:container_var_run_t:s0"
	ctxTmpfs    = "system_u:object_r:container_runtime_tmpfs_t:s0"
	ctxTLS      = "system_u:object_r:rke2_tls_t:s0"
	ctxLock     = "system_u:object_r:k3s_lock_t:s0"
	ctxData     = "system_u:object_r:k3s_data_t:s0"
	ctxRoot     = "system_u:object_r:k3s_root_t:s0"
	ctxNone     = "<<none>>"
)

type cmdCtx map[string]string

type configuration struct {
	distroName string
	cmdCtx
}

var conf = []configuration{
	{
		distroName: "rke2_centos7",
		cmdCtx: cmdCtx{
			cmdPrefix + " " + systemD + "/rke2*":                                   ctxUnitFile,
			cmdPrefix + " " + "/lib" + systemD + "/rke2*":                          ctxUnitFile,
			cmdPrefix + " " + usrLocal + "/lib" + systemD + "/rke2*":               ctxUnitFile,
			cmdPrefix + " " + usrBin + "/rke2":                                     ctxExec,
			cmdPrefix + " " + usrLocal + "/rke2":                                   ctxExec,
			cmdPrefix + " " + "/var/lib/cni " + ignoreDir:                          ctxVarLib,
			cmdPrefix + " " + "/var/lib/cni/* " + ignoreDir:                        ctxVarLib,
			cmdPrefix + " " + "/opt/cni " + ignoreDir:                              ctxFile,
			cmdPrefix + " " + "/opt/cni/* " + ignoreDir:                            ctxFile,
			cmdPrefix + " " + "/var/lib/kubelet/pods " + ignoreDir:                 ctxFile,
			cmdPrefix + " " + "/var/lib/kubelet/pods/* " + ignoreDir:               ctxFile,
			cmdPrefix + " " + rke2 + " " + ignoreDir:                               ctxVarLib,
			cmdPrefix + " " + rke2 + "/* " + ignoreDir:                             ctxVarLib,
			cmdPrefix + " " + rke2 + "/data":                                       ctxExec,
			cmdPrefix + " " + rke2 + "/data/*":                                     ctxExec,
			cmdPrefix + " " + rke2 + "/data/*/charts " + ignoreDir:                 ctxConfig,
			cmdPrefix + " " + rke2 + "/data/*/charts/*":                            ctxConfig,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots " + ignoreDir:  ctxShare,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots/ " + ignoreDir: ctxShare,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots/*/.*":          ctxNone,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/sandboxes " + ignoreDir:  ctxShare,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/sandboxes/ " + ignoreDir: ctxShare,
			cmdPrefix + " " + rke2 + "/server/logs " + ignoreDir:                   ctxLog,
			cmdPrefix + " " + rke2 + "/server/logs/ " + ignoreDir:                  ctxLog,
			cmdPrefix + " " + "/var/run/flannel " + ignoreDir:                      ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/flannel/* " + ignoreDir:                    ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s " + ignoreDir:                          ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/* " + ignoreDir:                        ctxRunTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm":          ctxTmpfs,
			cmdPrefix + " " + "/var/run/k3s/containerd/*/sandboxes/*/shm/*":        ctxTmpfs,
			cmdPrefix + " " + "/var/log/containers " + ignoreDir:                   ctxLog,
			cmdPrefix + " " + "/var/log/containers/* " + ignoreDir:                 ctxLog,
			cmdPrefix + " " + "/var/log/pods " + ignoreDir:                         ctxLog,
			cmdPrefix + " " + "/var/log/pods/* " + ignoreDir:                       ctxLog,
			cmdPrefix + " " + rke2 + "/server/tls " + ignoreDir:                    ctxTLS,
			cmdPrefix + " " + rke2 + "/server/tls/* " + ignoreDir:                  ctxTLS,
		},
	},
	{
		distroName: "rke2_centos8",
		cmdCtx: cmdCtx{
			cmdPrefix + " " + systemD + "/rke2*":                                    ctxUnitFile,
			cmdPrefix + " " + "/lib/systemd/system/rke2*":                           ctxUnitFile,
			cmdPrefix + " " + "/usr/local/lib/systemd/system/rke2*":                 ctxUnitFile,
			cmdPrefix + " " + usrBin + "/rke2":                                      ctxExec,
			cmdPrefix + " " + usrLocal + "/rke2":                                    ctxExec,
			cmdPrefix + " " + "/opt/cni " + ignoreDir:                               ctxFile,
			cmdPrefix + " " + "/opt/cni/* " + ignoreDir:                             ctxFile,
			cmdPrefix + " " + rke2:                                                  ctxVarLib,
			cmdPrefix + " " + rke2 + "/* " + ignoreDir:                              ctxVarLib,
			cmdPrefix + " " + rke2 + "/data " + ignoreDir:                           ctxExec,
			cmdPrefix + " " + rke2 + "/data/* " + ignoreDir:                         ctxExec,
			cmdPrefix + " " + rke2 + "/data/*/charts " + ignoreDir:                  ctxConfig,
			cmdPrefix + " " + rke2 + "/data/*/charts/* " + ignoreDir:                ctxConfig,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots " + ignoreDir:   ctxFile,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots/* " + ignoreDir: ctxFile,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/snapshots/*/.*":           ctxNone,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/sandboxes " + ignoreDir:   ctxShare,
			cmdPrefix + " " + rke2 + "/agent/containerd/*/sandboxes/* " + ignoreDir: ctxShare,
			cmdPrefix + " " + rke2 + "/server/logs " + ignoreDir:                    ctxLog,
			cmdPrefix + " " + rke2 + "/server/logs/* " + ignoreDir:                  ctxLog,
			cmdPrefix + " " + rke2 + "/server/tls " + ignoreDir:                     ctxTLS,
			cmdPrefix + " " + rke2 + "/server/tls/* " + ignoreDir:                   ctxTLS,
		},
	},
}
