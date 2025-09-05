package resources

import (
	"errors"
	"fmt"
	"strings"
)

func FindPath(name, ip string) (string, error) {
	if ip == "" {
		return "", errors.New("ip should not be empty")
	}

	if name == "" {
		return "", errors.New("name should not be empty")
	}

	// adding common paths to the environment variable PATH since in some os's not all paths are available.
	commonPaths := "/var/lib/rancher/rke2/bin:" +
		"/var/rancher/rke2/bin:" +
		"/var/lib/rancher:" +
		"/var/rancher:" +
		"/opt/rke2/bin:" +
		"/var/lib/rancher/k3s/bin:" +
		"/var/rancher/k3s/bin:" +
		"/opt/k3s/bin:" +
		"/usr/local/bin:" +
		"/usr/bin:"

	// adding the common paths to the PATH environment variable by sourcing it from a file.
	envFile := "find_path_env.sh"
	err := ExportEnvProfileNode([]string{ip}, map[string]string{"PATH": commonPaths}, envFile)
	if err != nil {
		return "", fmt.Errorf("failed to create environment file: %w", err)
	}

	sourcedCmds := []string{
		fmt.Sprintf(". /etc/profile.d/%s && which %s 2>/dev/null", envFile, name),
		fmt.Sprintf(". /etc/profile.d/%s && command -v %s 2>/dev/null", envFile, name),
		fmt.Sprintf(". /etc/profile.d/%s && type -p %s 2>/dev/null", envFile, name),
	}

	for _, cmd := range sourcedCmds {
		path, err := RunCommandOnNode(cmd, ip)
		if err != nil {
			LogLevel("warn", "Failed to find %s with sourced environment: %v", name, err)
			continue
		}
		if path != "" {
			return strings.TrimSpace(path), nil
		}
	}

	findCmd := fmt.Sprintf("find / -type f -executable -name %s 2>/dev/null | "+
		" grep -v data | sed 1q", name)
	fullPath, err := RunCommandOnNode(findCmd, ip)
	if err != nil {
		return "", fmt.Errorf("failed to run command %s: %w", findCmd, err)
	}
	if fullPath == "" {
		return "", fmt.Errorf("path for %s not found", name)
	}

	return strings.TrimSpace(fullPath), nil
}
