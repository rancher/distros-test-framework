package shared

import (
	"fmt"
	"strings"
)

// Product returns the distro product based on the config file
func Product() (string, error) {
	cfg, err := EnvConfig()
	if err != nil {
		return "", ReturnLogError("failed to get config path: %w\n", err)
	}

	if cfg.Product != "k3s" && cfg.Product != "rke2" {
		return "", ReturnLogError("unknown product")
	}

	return cfg.Product, nil
}

// ProductVersion return the version for a specific distro product
func ProductVersion(product string) (string, error) {
	if product != "rke2" && product != "k3s" {
		return "", ReturnLogError("unsupported product: %s\n", product)
	}
	ips := FetchNodeExternalIPs()

	cmd := fmt.Sprintf("%s -v", product)
	v, err := RunCommandOnNode(cmd, ips[0])
	if err != nil {
		return "", ReturnLogError("failed to get version for product: %s, error: %w\n", product, err)
	}

	return v, nil
}

// serviceName Get service name. Used to work with stop/start k3s/rke2 services
func serviceName(product, nodeType string) (string, error) {
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

func SystemCtlCmd(product, action, nodeType string) (string, error) {
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

	name, err := serviceName(product, nodeType)
	if err != nil {
		return "", ReturnLogError("error getting service name: %w\n", err)
	}

	return fmt.Sprintf("%s %s", sysctlPrefix, name), nil
}

// ManageService action:stop/start/restart/status product:rke2/k3s ips:ips array for nodeType:agent/server
func ManageService(product, action, nodeType string, ips []string) (string, error) {
	for _, ip := range ips {
		cmd, getError := SystemCtlCmd(product, action, nodeType)
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

func SecretEncryptOps(action, ip, product string) (string, error) {
	secretEncryptCmd := map[string]string{
		"status":      fmt.Sprintf("sudo %s secrets-encrypt status", product),
		"enable":      fmt.Sprintf("sudo %s secrets-encrypt enable", product),
		"disable":     fmt.Sprintf("sudo %s secrets-encrypt disable", product),
		"prepare":     fmt.Sprintf("sudo %s secrets-encrypt prepare", product),
		"rotate":      fmt.Sprintf("sudo %s secrets-encrypt rotate", product),
		"reencrypt":   fmt.Sprintf("sudo %s secrets-encrypt reencrypt", product),
		"rotate-keys": fmt.Sprintf("sudo %s secrets-encrypt rotate-keys", product),
	}

	secretsEncryptStdOut, err := RunCommandOnNode(secretEncryptCmd[action], ip)
	if err != nil {
		return "", ReturnLogError(fmt.Sprintf("FATAL: secrets-encryption %s action failed", action), err)
	}
	if strings.Contains(secretsEncryptStdOut, "fatal") {
		return "", ReturnLogError(fmt.Sprintf("FATAL: secrets-encryption %s action failed", action))
	}

	return secretsEncryptStdOut, nil
}
