package resources

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ExportEnvProfileNode creates a script with environment variables and exports them to the specified nodes.
func ExportEnvProfileNode(ips []string, vars map[string]string, filename string) error {
	if len(ips) == 0 {
		return errors.New("ips cannot be empty")
	}

	if vars == nil {
		return errors.New("vars cannot be empty")
	}

	if filename == "" {
		filename = "env_vars.sh"
	}

	var linesToAdd, varList []string
	linesToAdd = append(linesToAdd, "#!/usr/bin/env bash")

	for name, value := range vars {
		linesToAdd = append(linesToAdd, fmt.Sprintf("export %s=%s", name, value))
		varList = append(varList, fmt.Sprintf("%s=%s", name, value))
	}

	content := strings.Join(linesToAdd, "\n") + "\n"
	if strings.Contains(content, "/") {
		// remove any leading slashes from the filename.
		filename = strings.TrimPrefix(filename, "/")
	}
	tmp := "/tmp/" + filename
	dest := "/etc/profile.d/" + filename

	for _, ip := range ips {
		cmd := fmt.Sprintf(
			"sudo tee %s > /dev/null << EOF\n%s\nEOF\n"+
				"sudo chmod +x %s\n"+
				"sudo mv %s %s\n"+
				"sudo chmod 644 %s",
			tmp, content, tmp, tmp, dest, dest,
		)

		res, err := RunCommandOnNode(cmd, ip)
		if res != "" {
			return fmt.Errorf("failed to export environment on node %s: %s", ip, res)
		}
		if err != nil {
			return fmt.Errorf("failed to export environment on node %s: %w", ip, err)
		}
	}

	LogLevel("debug", "Environment variables exported to %s on %d nodes (%s): %s",
		dest, len(ips), filename, strings.Join(varList, ", "))

	return nil
}

func FindBinaries(ip string, binaries ...string) (map[string]string, error) {
	if ip == "" {
		return nil, errors.New("ip should not be empty")
	}

	if len(binaries) == 0 {
		// default binary if none provided.
		binaries = []string{"kubectl"}
	}

	binPaths := make(map[string]string)
	for _, bin := range binaries {
		path, err := FindPath(bin, ip)
		if err != nil {
			return nil, fmt.Errorf("failed to find binary %s: %w", bin, err)
		}

		binDir := filepath.Dir(path)
		binPaths[bin] = binDir
	}

	if len(binPaths) == 0 {
		return nil, fmt.Errorf("no requested binaries %s found on node %s", strings.Join(binaries, ", "), ip)
	}

	LogLevel("info", "Found binaries: %v", binPaths)

	return binPaths, nil
}

// GetJournalLogs returns the journal logs for a specific product.
func GetJournalLogs(level, ip, product string) string {
	if level == "" {
		LogLevel("warn", "level should not be empty")
		return ""
	}

	levels := map[string]bool{"info": true, "debug": true, "warn": true, "error": true, "fatal": true}
	if _, ok := levels[level]; !ok {
		LogLevel("warn", "Invalid log level: %s\n", level)
		return ""
	}

	cmd := fmt.Sprintf("sudo -i journalctl -u %s* --no-pager | grep -i '%s'", product, level)
	res, err := RunCommandOnNode(cmd, ip)
	if err != nil {
		LogLevel("warn", "failed to get journal logs for product: %s, error: %v\n", product, err)
		return ""
	}

	return fmt.Sprintf("Journal logs for product: %s (level: %s):\n%s", product, level, res)
}
