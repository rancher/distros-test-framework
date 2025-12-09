package resources

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

// RunScp copies files from local to remote host based on a list of local and remote paths.
func RunScp(c *driver.Cluster, ip string, localPaths, remotePaths []string) error {
	if ip == "" {
		return ReturnLogError("ip is needed.\n")
	}

	if c.Config.Product != "rke2" && c.Config.Product != "k3s" {
		return ReturnLogError("unsupported product: %s\n", c.Config.Product)
	}

	if len(localPaths) != len(remotePaths) {
		return ReturnLogError("the number of local paths and remote paths must be the same\n")
	}

	for i, localPath := range localPaths {
		remotePath := remotePaths[i]
		scp := fmt.Sprintf(
			"ssh-keyscan %[1]s >> /root/.ssh/known_hosts && "+
				"chmod 400 %[2]s && scp -i %[2]s %[3]s %[4]s@%[1]s:%[5]s",
			ip,
			c.Aws.AccessKeyID,
			localPath,
			c.SSH.User,
			remotePath,
		)

		LogLevel("debug", "Running scp command: %s\n", scp)
		res, cmdErr := RunCommandHost(scp)
		if res != "" {
			LogLevel("warn", "SCP output: %s\n", res)
		}
		if cmdErr != nil {
			LogLevel("error", "Failed to run scp: %v\n", cmdErr)
			return cmdErr
		}

		chmod := "sudo chmod +wx " + remotePath
		_, cmdErr = RunCommandOnNode(chmod, ip)
		if cmdErr != nil {
			LogLevel("error", "Failed to run chmod: %v\n", cmdErr)
			return cmdErr
		}
	}

	return nil
}

func CopyFileToRemoteNode(ip, username, pem_file_path string, localFilePath, remoteFilePath string) error {
	file, err := os.Open(localFilePath)
	if err != nil {
		log.Fatalf("failed to open local file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var cmd string

	cmd = fmt.Sprintf("[ -f \"%s\" ] && rm -rf \"%s\"", remoteFilePath, remoteFilePath)
	_, cmdErr := RunCommandOnNode(cmd, ip)
	if cmdErr != nil {
		LogLevel("debug", "error removing existing remote file: %v\n", cmdErr)
	}

	for scanner.Scan() {
		line := scanner.Text()
		// Implement remote write logic here
		cmd = fmt.Sprintf("echo \"%s\" >> %s", line, remoteFilePath)
		_, cmdErr := RunCommandOnNode(cmd, ip)
		if cmdErr != nil {
			LogLevel("error", "error running cmd: %v\n", cmdErr)
			return ReturnLogError("failed to run remote write command\n")
		}
	}
	cmd = "cat " + remoteFilePath
	res, cmdErr := RunCommandOnNode(cmd, ip)
	if cmdErr != nil {
		LogLevel("error", "error verifying remote file: %v\n", cmdErr)
		return ReturnLogError("failed to verify remote file\n")
	}
	LogLevel("debug", "Remote file content:\n%s\n", res)

	if err := scanner.Err(); err != nil {
		LogLevel("error", "error while reading local file: %v\n", err)
		return ReturnLogError("error while reading local file\n")
		// log.Fatalf("error while scanning local file: %v", err)
	}

	return nil
}

// MountBind mounts a directory to another directory on the given node IP addresses.
func MountBind(ips []string, dir, mountPoint string) error {
	if ips == nil {
		return errors.New("ips should not be empty")
	}

	if dir == "" || mountPoint == "" {
		return errors.New(" dir and mountPoint should not be empty")
	}

	if !strings.HasPrefix(dir, "/") || !strings.HasPrefix(mountPoint, "/") {
		return fmt.Errorf("dir and mountPoint should  be absolute paths %s and %s", dir, mountPoint)
	}

	LogLevel("info", "Mounting %s to %s on nodes: %v\n", dir, mountPoint, ips)

	cmd := "sudo mount --bind " + dir + " " + mountPoint
	for _, ip := range ips {
		res, err := RunCommandOnNode(cmd, ip)
		if err != nil {
			return fmt.Errorf("failed to run command: %s, error: %w", cmd, err)
		}
		res = strings.TrimSpace(res)
		if res != "" {
			return fmt.Errorf("failed to run command: %s, error: %s", cmd, res)
		}
	}

	return nil
}
