package shared

import (
	"fmt"

	"github.com/rancher/distros-test-framework/config"
)

// GetProductObject returns the distro "Product" object based on the config file
func GetProductObject() (Product, error) {
	cfgPath, err := EnvDir(".")
	if err != nil {
		return "", ReturnLogError("failed to get config path: %v\n", err)
	}

	cfg, err := config.AddConfigEnv(cfgPath)
	if err != nil {
		return "", ReturnLogError("failed to get config: %v\n", err)
	}

	var product Product
	if cfg.Product == "k3s" {
		product = K3S
	}
	if cfg.Product == "rke2" {
		product = RKE2
	}

	return product, nil
}

type Product string

const (
	K3S  Product = "k3s"
	RKE2 Product = "rke2"
)

// getServiceName Get service name. Used to work with stop/start k3s/rke2 services
func (p Product) getServiceName(nodeType string) (string, error) {
	serviceNameMap := map[string]string{
		"k3s-server":  "k3s",
		"k3s-agent":   "k3s-agent",
		"rke2-server": "rke2-server",
		"rke2-agent":  "rke2-agent",
	}
	serviceName, ok := serviceNameMap[fmt.Sprintf("%s-%s", p, nodeType)]
	if !ok {
		return "", ReturnLogError("nodeType needs to be one of: server | agent")
	}

	return serviceName, nil
}

func (p Product) GetSystemCtlCmd(action, nodeType string) (string, error) {
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

	serviceName, err := p.getServiceName(nodeType)
	if err != nil {
		return "", ReturnLogError("error getting service name")
	}

	return fmt.Sprintf("%s %s", sysctlPrefix, serviceName), nil
}

// ManageService action:stop/start/restart/status product:rke2/k3s ips:ips array for nodeType:agent/server
func (p Product) ManageService(action, nodeType string, ips []string) (string, error) {
	for _, ip := range ips {
		cmd, getError := p.GetSystemCtlCmd(action, nodeType)
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
func (p Product) CertRotate(ips []string) (string, error) {
	for _, ip := range ips {
		cmd := fmt.Sprintf("sudo %s certificate rotate", p)
		_, err := RunCommandOnNode(cmd, ip)
		if err != nil {
			return ip, err
		}
	}

	return "", nil
}
