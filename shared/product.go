package shared

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/customflag"
)

// Product returns the distro product and its current version.
func Product() (product, version string, err error) {
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

	cmd := path + " -v"
	v, err := RunCommandOnNode(cmd, ips[0])
	if err != nil {
		return "", ReturnLogError("failed to get version for product: %s, error: %w\n", product, err)
	}

	return v, nil
}

// productService gets the service name for a specific distro product and nodeType.
func productService(product, nodeType string) (string, error) {
	if nodeType == "" {
		return "", fmt.Errorf("nodeType required for %s service", product)
	}

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
func CertRotate(product, ip string) error {
	product = "-E env \"PATH=$PATH:/usr/local/bin:/usr/bin\"  " + product
	if ip == "" {
		return ReturnLogError("IP address is required")
	}

	cmd := fmt.Sprintf("sudo %s certificate rotate", product)
	certRotateOut, err := RunCommandOnNode(cmd, ip)
	if err != nil {
		return ReturnLogError("certificate rotate failed on %s", ip)
	}

	LogLevel("debug", "On %s, cert rotate output:\n %s", ip, certRotateOut)

	return nil
}

func SecretEncryptOps(action, ip, product string) (string, error) {
	product = "-E env \"PATH=$PATH:/usr/local/bin:/usr/bin\" " + product
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
		return "", ReturnLogError("secrets-encryption %s action failed", action)
	}
	LogLevel("debug", "%s output:\n %s", action, secretsEncryptStdOut)

	return secretsEncryptStdOut, nil
}

func GetInstallCmd(product, installType, nodeType string) string {
	var installFlag string
	var installCmd string

	if strings.HasPrefix(installType, "v") {
		installFlag = fmt.Sprintf("INSTALL_%s_VERSION=%s", strings.ToUpper(product), installType)
	} else {
		installFlag = fmt.Sprintf("INSTALL_%s_COMMIT=%s", strings.ToUpper(product), installType)
	}

	channel := getChannel(product)
	installMethod := fmt.Sprintf("INSTALL_%s_METHOD=%s", strings.ToUpper(product), os.Getenv("install_method"))
	installCmd = fmt.Sprintf("curl -sfL https://get.%s.io | sudo %%s %%s %s sh -s - %s", product, installMethod, nodeType)

	return fmt.Sprintf(installCmd, installFlag, channel)
}

func getChannel(product string) string {
	defaultChannel := fmt.Sprintf("INSTALL_%s_CHANNEL=%s", strings.ToUpper(product), "testing")

	if customflag.ServiceFlag.Channel.String() != "" {
		return fmt.Sprintf("INSTALL_%s_CHANNEL=%s", strings.ToUpper(product),
			customflag.ServiceFlag.Channel.String())
	}

	return defaultChannel
}

func ManageProductCleanup(product, nodeType, ip string, actions ...string) error {
	if product != "k3s" && product != "rke2" {
		return fmt.Errorf("unsupported product: %s", product)
	}

	if len(actions) == 0 {
		return fmt.Errorf("no actions specified for %s cleanup", product)
	}

	for _, action := range actions {
		switch action {
		case "uninstall":
			uninstallScript := product + "-uninstall.sh"
			if product == "k3s" && nodeType == "agent" {
				uninstallScript = "k3s-agent-uninstall.sh"
			}
			err := execAction(product, uninstallScript, ip)
			if err != nil {
				return fmt.Errorf("%v uninstall failed for %s-%s", err, product, nodeType)
			}

			LogLevel("info", "%s completed successfully on %s ip:%s", uninstallScript, nodeType, ip)

		case "killall":
			killAllScript := product + "-killall.sh"
			err := execAction(product, killAllScript, ip)
			if err != nil {
				return fmt.Errorf("killall failed for %s-%s", product, nodeType)
			}

			LogLevel("info", "%s completed successfully on %s ip:%s", killAllScript, nodeType, ip)

		default:
			return fmt.Errorf("unsupported action: %s", action)
		}
	}

	return nil
}

func execAction(product, script, ip string) error {
	execPath, err := FindPath(script, ip)
	if err != nil {
		return fmt.Errorf("failed to find %s script for %s: %w", script, product, err)
	}

	LogLevel("info", "execPath path: %s", execPath)

	cmd := "sudo " + execPath
	res, execErr := RunCommandOnNode(cmd, ip)
	// here we should check if its not empty because scripts ran here, will always return output.
	if strings.TrimSpace(res) == "" {
		return fmt.Errorf("failed to run command: %s", execPath)
	}
	if execErr != nil {
		return fmt.Errorf("failed to run command: %s, error: %w", execPath, execErr)
	}

	return nil
}
