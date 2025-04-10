package support

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/shared"
)

type ScriptArgs struct {
	NodeOS          string
	FQDN            string
	MasterIP        string
	Token           string
	PublicIPv4      string
	PrivateIPv4     string
	PublicIPv6      string
	InstallChannel  string
	InstallMode     string
	InstallMethod   string
	InstallVersion  string
	ETCDOnlyNodes   string
	DatastoreType   string
	DatastoreEP     string
	ServerFlags     string
	WorkerFlags     string
	RHELUsername    string
	RHELPassword    string
	InstallOrEnable string
}

func buildInstallCmd(cluster *shared.Cluster, nodeType, ip string) (cmd string) {
	product := cluster.Config.Product
	args := ScriptArgs{
		NodeOS:          os.Getenv("node_os"),
		FQDN:            cluster.FQDN,
		MasterIP:        cluster.ServerIPs[0],
		Token:           token,
		PublicIPv4:      ip,
		PrivateIPv4:     "",
		PublicIPv6:      "",
		InstallChannel:  os.Getenv("install_channel"),
		InstallMethod:   os.Getenv("install_method"),
		InstallMode:     os.Getenv("install_mode"),
		InstallVersion:  os.Getenv("install_version"),
		ETCDOnlyNodes:   os.Getenv("etcd_only_nodes"),
		DatastoreType:   os.Getenv("datastore_type"),
		DatastoreEP:     os.Getenv("datastore_endpoint"),
		ServerFlags:     os.Getenv("server_flags"),
		WorkerFlags:     os.Getenv("worker_flags"),
		RHELUsername:    os.Getenv("username"),
		RHELPassword:    os.Getenv("password"),
		InstallOrEnable: "both",
	}
	if strings.Contains(ip, ":") {
		args.PublicIPv4 = ""
		args.PublicIPv6 = ip
	}
	if product == "k3s" {
		cmd = buildK3SCmd(&args, product, nodeType)
	}
	if product == "rke2" {
		cmd = buildRKE2Cmd(&args, product, nodeType)
	}

	return cmd
}

func buildK3SCmd(args *ScriptArgs, product, nodeType string) (cmd string) {
	var script string
	if nodeType == "master" && token == "" {
		script = fmt.Sprintf("%v_%v.sh", product, nodeType)
		cmd = fmt.Sprintf("sudo chmod +x %v; ", script)
		cmd += fmt.Sprintf(
			`sudo ./%v "%v" "%v" "%v" "%v" "%v" "%v" "%v" `+
				`"%v" "%v" "%v" "%v" "%v" "%v" "%v" "%v"`,
			script, args.NodeOS, args.FQDN, args.PublicIPv4, args.PrivateIPv4,
			args.PublicIPv6, args.InstallMode, args.InstallVersion, args.InstallChannel,
			args.ETCDOnlyNodes, args.DatastoreType, args.DatastoreEP, args.ServerFlags,
			args.RHELUsername, args.RHELPassword, args.InstallOrEnable,
		)
	}
	if nodeType == "master" && token != "" {
		script = fmt.Sprintf("join_%v_%v.sh", product, nodeType)
		cmd = fmt.Sprintf("sudo chmod +x %v; ", script)
		cmd += fmt.Sprintf(
			`sudo ./%v "%v" "%v" "%v" "%v" "%v" "%v" "%v" `+
				`"%v" "%v" "%v" "%v" "%v" "%v" "%v" "%v" "%v"`,
			script, args.NodeOS, args.FQDN, args.MasterIP, args.Token,
			args.PublicIPv4, args.PrivateIPv4, args.PublicIPv6,
			args.InstallMode, args.InstallVersion, args.InstallChannel,
			args.DatastoreType, args.DatastoreEP, args.ServerFlags,
			args.RHELUsername, args.RHELPassword, args.InstallOrEnable,
		)
	}
	if nodeType == "agent" {
		script = fmt.Sprintf("join_%v_%v.sh", product, nodeType)
		cmd = fmt.Sprintf("sudo chmod +x %v; ", script)
		cmd += fmt.Sprintf(
			`sudo ./%v "%v" "%v" "%v" "%v" "%v" "%v" `+
				`"%v" "%v" "%v" "%v" "%v" "%v" "%v"`,
			script, args.NodeOS, args.MasterIP, args.Token,
			args.PublicIPv4, args.PrivateIPv4, args.PublicIPv6,
			args.InstallMode, args.InstallVersion, args.InstallChannel,
			args.WorkerFlags, args.RHELUsername, args.RHELPassword,
			args.InstallOrEnable,
		)
	}

	return cmd
}

func buildRKE2Cmd(args *ScriptArgs, product, nodeType string) (cmd string) {
	var script string
	if nodeType == "master" && token == "" {
		script = fmt.Sprintf("%v_%v.sh", product, nodeType)
		cmd = fmt.Sprintf("sudo chmod +x %v; ", script)
		cmd += fmt.Sprintf(
			`sudo ./%v "%v" "%v" "%v" "%v" "%v" "%v" "%v" `+
				`"%v" "%v" "%v" "%v" "%v" "%v" "%v" "%v"`,
			script, args.NodeOS, args.FQDN, args.PublicIPv4, args.PrivateIPv4,
			args.PublicIPv6, args.InstallMode, args.InstallVersion, args.InstallChannel,
			args.InstallMethod, args.DatastoreType, args.DatastoreEP, args.ServerFlags,
			args.RHELUsername, args.RHELPassword, args.InstallOrEnable,
		)
	}
	if nodeType == "master" && token != "" {
		script = fmt.Sprintf("join_%v_%v.sh", product, nodeType)
		cmd = fmt.Sprintf("sudo chmod +x %v; ", script)
		cmd += fmt.Sprintf(
			`sudo ./%v "%v" "%v" "%v" "%v" "%v" "%v" "%v" "%v"`+
				`"%v" "%v" "%v" "%v" "%v" "%v" "%v" "%v" "%v"`,
			script, args.NodeOS, args.FQDN, args.MasterIP, args.Token,
			args.PublicIPv4, args.PrivateIPv4, args.PublicIPv6,
			args.InstallMode, args.InstallVersion, args.InstallChannel,
			args.InstallMethod, args.DatastoreType, args.DatastoreEP,
			args.ServerFlags, args.RHELUsername, args.RHELPassword,
			args.InstallOrEnable,
		)
	}
	if nodeType == "agent" {
		script = fmt.Sprintf("join_%v_%v.sh", product, nodeType)
		cmd = fmt.Sprintf("sudo chmod +x %v; ", script)
		cmd += fmt.Sprintf(
			`sudo ./%v "%v" "%v" "%v" "%v" "%v" "%v" `+
				`"%v" "%v" "%v" "%v" "%v" "%v" "%v" "%v"`,
			script, args.NodeOS, args.MasterIP, args.Token,
			args.PublicIPv4, args.PrivateIPv4, args.PublicIPv6,
			args.InstallMode, args.InstallVersion, args.InstallChannel,
			args.InstallMethod, args.WorkerFlags,
			args.RHELUsername, args.RHELPassword,
			args.InstallOrEnable,
		)
	}

	return cmd
}
