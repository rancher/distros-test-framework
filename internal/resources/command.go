package resources

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// RunCommandOnNode executes a command on the node SSH.
func RunCommandOnNode(cmd, ip string) (string, error) {
	if cmd == "" {
		return "", ReturnLogError("cmd should not be empty")
	}

	host := ip + ":22"
	conn, err := getOrDialSSH(host)
	if err != nil {
		return "", fmt.Errorf("failed to connect to host %s: %v", host, err)
	}

	stdout, stderr, err := runsshCommand(cmd, conn)
	if err != nil && !strings.Contains(stderr, "restart") {
		return "", fmt.Errorf(
			"command: %s failed on run ssh: %s with error: %w\n, stderr: %v",
			cmd,
			ip,
			err,
			stderr,
		)
	}

	stdout = strings.TrimSpace(stdout)
	stderr = strings.TrimSpace(stderr)

	cleanedStderr := strings.ReplaceAll(stderr, "\n", "")
	cleanedStderr = strings.ReplaceAll(cleanedStderr, "\t", "")

	if cleanedStderr != "" && (!strings.Contains(stderr, "exited") || !strings.Contains(cleanedStderr, "1") ||
		!strings.Contains(cleanedStderr, "2")) {
		return cleanedStderr, nil
	} else if cleanedStderr != "" {
		return "", fmt.Errorf("command: %s failed with error: %v", cmd, stderr)
	}

	return stdout, err
}

// RunCommandHost executes a command on the host.
func RunCommandHost(cmds ...string) (string, error) {
	if cmds == nil {
		return "", ReturnLogError("should send at least one command")
	}

	var output, errOut bytes.Buffer
	for _, cmd := range cmds {
		if cmd == "" {
			return "", ReturnLogError("cmd should not be empty")
		}

		c := exec.Command("bash", "-c", cmd)
		c.Stdout = &output
		c.Stderr = &errOut

		err := c.Run()
		if err != nil {
			LogLevel("error", "Command '%s' failed with error: %v\n %v", cmd, err, c.Stderr.(*bytes.Buffer).String())
			return c.Stderr.(*bytes.Buffer).String(), err
		}
	}

	return output.String(), nil
}

// JoinCommands joins the first command with some arg.
//
// That could separators like ";" , | , "&&" etc.
//
// Example:
// "kubectl get nodes -o wide | grep IMAGES"
//
// should be called like this:
//
// "kubectl get nodes -o wide : | grep IMAGES".
func JoinCommands(cmd, kubeconfigFlag string) string {
	cmds := strings.Split(cmd, ":")
	joinedCmd := cmds[0] + kubeconfigFlag

	if len(cmds) > 1 {
		secondCmd := strings.Join(cmds[1:], ",")
		joinedCmd += " " + secondCmd
	}

	return joinedCmd
}
