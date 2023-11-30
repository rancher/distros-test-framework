package shared

import (
	"fmt"

	"github.com/rancher/distros-test-framework/config"
)

// GetProduct returns the distro product based on the config file
func GetProduct() (string, error) {
	cfgPath, err := EnvDir("shared")
	if err != nil {
		return "", ReturnLogError("failed to get config path: %v\n", err)
	}

	cfg, err := config.AddConfigEnv(cfgPath)
	if err != nil {
		return "", ReturnLogError("failed to get config: %v\n", err)
	}
	if cfg.Product != "k3s" && cfg.Product != "rke2" {
		return "", ReturnLogError("unknown product")
	}

	return cfg.Product, nil
}

// GetProductVersion return the version for a specific distro product
func GetProductVersion(product string) (string, error) {
	if product != "rke2" && product != "k3s" {
		return "", ReturnLogError("unsupported product: %s\n", product)
	}
	version, err := getVersion(product + " -v")
	if err != nil {
		return "", ReturnLogError("failed to get version for product: %s, error: %v\n", product, err)
	}

	return version, nil
}

// getVersion returns the rke2 or k3s version
func getVersion(cmd string) (string, error) {
	var res string
	var err error
	ips := FetchNodeExternalIP()
	for _, ip := range ips {
		res, err = RunCommandOnNode(cmd, ip)
		if err != nil {
			return "", ReturnLogError("failed to run command on node: %v\n", err)
		}
	}

	return res, nil
}

// getServiceName Get service name. Used to work with stop/start k3s/rke2 services
func getServiceName(product, nodeType string) (string, error) {
	serviceNameMap := map[string]string{
		"k3s-server":  "k3s",
		"k3s-agent":   "k3s-agent",
		"rke2-server": "rke2-server",
		"rke2-agent":  "rke2-agent",
	}
	serviceName, ok := serviceNameMap[fmt.Sprintf("%s-%s", product, nodeType)]
	if !ok {
		return "", ReturnLogError("nodeType needs to be one of: server | agent")
	}

	return serviceName, nil
}

func GetSystemCtlCmd(product, action, nodeType string) (string, error) {
	systemctlCmdMap := map[string]string{
		"stop":    "sudo systemctl --no-block stop",
		"start":   "sudo systemctl --no-block start",
		"restart": "sudo systemctl --no-block restart",
		"status":  "sudo systemctl --no-block status",
	}

	sysctlPrefix, ok := systemctlCmdMap[action]
	if !ok {
		return "", ReturnLogError("action value should be: start | stop | restart | status")
	}

	serviceName, err := getServiceName(product, nodeType)
	if err != nil {
		return "", ReturnLogError("error getting service name")
	}

	return fmt.Sprintf("%s %s", sysctlPrefix, serviceName), nil
}

// ManageService action:stop/start/restart/status product:rke2/k3s ips:ips array for nodeType:agent/server
func ManageService(product, action, nodeType string, ips []string) (string, error) {
	for _, ip := range ips {
		cmd, getError := GetSystemCtlCmd(product, action, nodeType)
		if getError != nil {
			return ip, getError
		}
		_, err := RunCommandOnNode(cmd, ip)
		if err != nil {
			return ip, err
		}
	}

	return "", nil
}

// CertRotate certificate rotate for k3s or rke2
func CertRotate(product string, ips []string) (string, error) {
	for _, ip := range ips {
		cmd := fmt.Sprintf("sudo %s certificate rotate", product)
		_, err := RunCommandOnNode(cmd, ip)
		if err != nil {
			return ip, err
		}
	}

	return "", nil
}
