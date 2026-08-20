package resources

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
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

// RunCommandHost executes shell command lines on the host, without a
// deadline — long operations (e.g. `sonobuoy run --wait`) own their runtime.
func RunCommandHost(cmds ...string) (string, error) {
	return RunCommandHostContext(context.Background(), cmds...)
}

// RunCommandHostWithTimeout executes shell command lines on the host,
// canceling the whole process group when timeout elapses.
func RunCommandHostWithTimeout(timeout time.Duration, cmds ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return RunCommandHostContext(ctx, cmds...)
}

// RunCommandHostContext executes shell command lines on the host under ctx.
func RunCommandHostContext(ctx context.Context, cmds ...string) (string, error) {
	if cmds == nil {
		return "", ReturnLogError("should send at least one command")
	}

	var output, errOut bytes.Buffer
	for _, cmd := range cmds {
		if cmd == "" {
			return "", ReturnLogError("cmd should not be empty")
		}

		c := exec.CommandContext(ctx, "bash", "-c", cmd)
		c.Stdout = &output
		c.Stderr = &errOut
		hardenCancellation(c)

		err := c.Run()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				err = fmt.Errorf("command canceled (%w): %w", ctxErr, err)
			}
			LogLevel("error", "Command '%s' failed with error: %v\n %v", cmd, err, errOut.String())

			return errOut.String(), err
		}
	}

	return output.String(), nil
}

// RunHostArgs executes a single binary with separated arguments — no shell is
// involved, so argument values can never be interpreted as shell syntax.
func RunHostArgs(name string, args ...string) (string, error) {
	return RunHostArgsContext(context.Background(), name, args...)
}

// RunHostArgsContext is RunHostArgs under a caller-controlled context.
func RunHostArgsContext(ctx context.Context, name string, args ...string) (string, error) {
	if name == "" {
		return "", ReturnLogError("binary name should not be empty")
	}

	var output, errOut bytes.Buffer
	c := exec.CommandContext(ctx, name, args...)
	c.Stdout = &output
	c.Stderr = &errOut
	hardenCancellation(c)

	if err := c.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = fmt.Errorf("command canceled (%w): %w", ctxErr, err)
		}
		LogLevel("error", "Command %s %v failed with error: %v\n %v", name, args, err, errOut.String())

		return errOut.String(), err
	}

	return output.String(), nil
}

// hardenCancellation makes cancellation reach the whole process group:
// killing only bash would leave pipeline children alive holding the pipes,
// and Run() would keep blocking after the deadline.
func hardenCancellation(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
	c.WaitDelay = 10 * time.Second
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
