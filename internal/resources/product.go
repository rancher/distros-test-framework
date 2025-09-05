package resources

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
)

// Product returns the distro product and its current version.
func Product() (product, version string, err error) {
	p := os.Getenv("ENV_PRODUCT")
	if p != "k3s" && p != "rke2" {
		return "", "", ReturnLogError("unknown product")
	}

	v, vErr := productVersion(p)
	if vErr != nil {
		return "", "", ReturnLogError("failed to get version for product: %s, error: %w\n", p, vErr)
	}

	return p, v, nil
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
		return "", ReturnLogError("%s secrets-encrypt %s failed on node: %s!\n%v", product, action, ip, err)
	}
	if strings.Contains(secretsEncryptStdOut, "fatal") {
		return "", ReturnLogError("secrets-encryption %s action failed", action)
	}
	LogLevel("debug", "%s secrets-encrypt %s output on node: %s\n %s", product, action, ip, secretsEncryptStdOut)

	return secretsEncryptStdOut, nil
}

func GetInstallCmd(cluster *Cluster, installType, nodeType string) string {
	var installFlag string
	var installCmd string

	product := cluster.Config.Product
	nodeOS := cluster.NodeOS

	channel := getChannelFlag(product)

	if strings.HasPrefix(installType, "v") {
		installFlag = fmt.Sprintf("INSTALL_%s_VERSION=%s", strings.ToUpper(product), installType)
	} else {
		installFlag = fmt.Sprintf("INSTALL_%s_COMMIT=%s", strings.ToUpper(product), installType)
	}

	installMethodValue := os.Getenv("install_method")
	installMethod := ""
	if installMethodValue != "" {
		installMethod = fmt.Sprintf("INSTALL_%s_METHOD=%s", strings.ToUpper(product), installMethodValue)
		installCmd = fmt.Sprintf("curl -sfL https://get.%s.io | sudo %%s %%s %%s sh -s - %s", product, nodeType)
		LogLevel("debug", "installCmd: %s installFlag: %s channel: %s installMethod: %s",
			installCmd, installFlag, channel, installMethod)

		return fmt.Sprintf(installCmd, installFlag, channel, installMethod)
	}

	if nodeOS == "slemicro" && strings.EqualFold(product, "k3s") {
		skipEnable := fmt.Sprintf("INSTALL_%s_SKIP_ENABLE=true", strings.ToUpper(product))
		installCmd = fmt.Sprintf("curl -sfL https://get.%s.io | sudo %%s %%s %%s sh -s - %s", product, nodeType)
		LogLevel("debug", "installCmd: %s installFlag: %s channel: %s skipEnable: %s",
			installCmd, installFlag, channel, skipEnable)

		return fmt.Sprintf(installCmd, installFlag, channel, skipEnable)
	}

	installCmd = fmt.Sprintf("curl -sfL https://get.%s.io | sudo %%s %%s  sh -s - %s", product, nodeType)
	LogLevel("debug", "installCmd: %s installFlag: %s channel: %s", installCmd, installFlag, channel)

	return fmt.Sprintf(installCmd, installFlag, channel)
}

func getChannelFlag(product string) string {
	defaultChannel := fmt.Sprintf("INSTALL_%s_CHANNEL=%s", strings.ToUpper(product), "testing")

	if customflag.ServiceFlag.Channel.String() != "" {
		return fmt.Sprintf("INSTALL_%s_CHANNEL=%s", strings.ToUpper(product),
			customflag.ServiceFlag.Channel.String())
	}

	return defaultChannel
}

// ManageProductCleanup performs cleanup actions for k3s or rke2.
// It supports uninstall and killall actions.
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
	if execErr != nil {
		return fmt.Errorf("failed to run command: %s, error: %w\nresponse: %v", execPath, execErr, res)
	}

	return nil
}
