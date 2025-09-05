package resources

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/internal/logging"
)

// RunScp copies files from local to remote host based on a list of local and remote paths.
func RunScp(c *Cluster, ip string, localPaths, remotePaths []string) error {
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
