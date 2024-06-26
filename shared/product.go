package shared

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/config"
)

// Product returns the distro product and its current version.
func Product() (product string, version string, err error) {
	cfg, err := config.AddEnv()
	if err != nil {
		return "", "", ReturnLogError("failed to get config path: %w\n", err)
	}

	if cfg.Product != "k3s" && cfg.Product != "rke2" {
		return "", "", ReturnLogError("unknown product")
	}

	v, vErr := productVersion(cfg.Product)
	if vErr != nil {
		return "", "", ReturnLogError("failed to get version for product: %s, error: %w\n", cfg.Product, vErr)
	}

	return cfg.Product, v, nil
}

// productVersion return the version for a specific distro product based on current installation through node external IP.
func productVersion(product string) (string, error) {
	ips := FetchNodeExternalIPs()

	path, findErr := FindPath(product, ips[0])
	if findErr != nil {
		return "", ReturnLogError("failed to find path for product: %s, error: %w\n", product, findErr)
	}

	cmd := fmt.Sprintf("%s -v", path)
	v, err := RunCommandOnNode(cmd, ips[0])
	if err != nil {
		return "", ReturnLogError("failed to get version for product: %s, error: %w\n", product, err)
	}

	return v, nil
}

// ManageService action:stop/start/restart/status product:rke2/k3s ips:ips array for nodeType:agent/server.
func ManageService(product, action, nodeType string, ips []string) (string, error) {
	if len(ips) == 0 {
		return "", ReturnLogError("ips string array cannot be empty")
	}

	for _, ip := range ips {
		cmd, getError := SystemCtlCmd(product, action, nodeType)
		if getError != nil {
			return ip, getError
		}
		manageServiceOut, err := RunCommandOnNode(cmd, ip)
		if err != nil {
			return ip, err
		}
		if manageServiceOut != "" {
			LogLevel("debug", "service %s output: \n %s", action, manageServiceOut)
		}
	}

	return "", nil
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

// serviceName Get service name. Used to work with stop/start k3s/rke2 services.
func serviceName(product, nodeType string) (string, error) {
	serviceNameMap := map[string]string{
		"k3s-server":  "k3s",
		"k3s-agent":   "k3s-agent",
		"rke2-server": "rke2-server",
		"rke2-agent":  "rke2-agent",
	}

	svcName, ok := serviceNameMap[fmt.Sprintf("%s-%s", product, nodeType)]
	if !ok {
		return "", ReturnLogError("nodeType needs to be one of: server | agent")
	}

	return svcName, nil
}

// CertRotate certificate rotate for k3s or rke2.
func CertRotate(product string, ips []string) (string, error) {
	product = fmt.Sprintf("-E env \"PATH=$PATH:/usr/local/bin:/usr/bin\" %s", product)
	if len(ips) == 0 {
		return "", ReturnLogError("ips string array cannot be empty")
	}

	for _, ip := range ips {
		cmd := fmt.Sprintf("sudo %s certificate rotate", product)
		certRotateOut, err := RunCommandOnNode(cmd, ip)
		if err != nil {
			return ip, err
		}
		LogLevel("debug", "On %s, cert rotate output:\n %s", ip, certRotateOut)
	}

	return "", nil
}

func SecretEncryptOps(action, ip, product string) (string, error) {
	product = fmt.Sprintf("-E env \"PATH=$PATH:/usr/local/bin:/usr/bin\" %s", product)
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
		return "", ReturnLogError(fmt.Sprintf("secrets-encryption %s action failed", action), err)
	}
	if strings.Contains(secretsEncryptStdOut, "fatal") {
		return "", ReturnLogError(fmt.Sprintf("secrets-encryption %s action failed", action))
	}
	LogLevel("debug", "%s output:\n %s", action, secretsEncryptStdOut)

	return secretsEncryptStdOut, nil
}
