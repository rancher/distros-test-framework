package resources

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type sshConn struct {
	sync.Mutex
	connClient map[string]*ssh.Client
}

var connPool = sshConn{connClient: make(map[string]*ssh.Client)}

// RetryCfg is the configuration for retrying commands.
// Attempts: total attempts for the command.
// Delay: delay before 1st retry.
// DelayMultiplier: delay multiplier for each retry if needed.
// RetryableExitCodes: e.g. []int{1, 2, 255}.
// RetryableErrorSubString: error substrings that MAY retry.
// NonRetryableErrorSubString: error substrings that MUST stop retrying.
type RetryCfg struct {
	Attempts                   int
	Delay                      time.Duration
	DelayMultiplier            float64
	RetryableExitCodes         []int
	RetryableErrorSubString    []string
	NonRetryableErrorSubString []string
}

var defaultRetryCfg = RetryCfg{
	Attempts:        3,
	Delay:           2 * time.Second,
	DelayMultiplier: 1.0,
	RetryableExitCodes: []int{
		1,
		255,
	},
	RetryableErrorSubString: []string{
		"exit status 1",
		"without exit status",
		"connection refused",
		"command timed out",
		"connect: connection reset by peer",
		"connect: operation timed out",
		"exit signal",
	},
	NonRetryableErrorSubString: []string{
		"Permission denied",
		"Host key verification failed",
		"invalid argument",
	},
}

func CmdNodeRetryCfg() RetryCfg {
	return defaultRetryCfg
}

// RunCommandOnNodeWithRetry runs a command on a node with error retry config logic.
func RunCommandOnNodeWithRetry(cmd, ip string, cfg *RetryCfg) (string, error) {
	LogLevel("debug", "Running command on node with ssh error retry %s: %s\ncfg: %+v\n", ip, cmd, cfg)

	if cfg == nil {
		tmp := defaultRetryCfg
		cfg = &tmp
	}

	if cfg.Attempts < 1 {
		return "", fmt.Errorf("invalid attempts: %d", cfg.Attempts)
	}

	if ip == "" {
		return "", errors.New("ip address is empty")
	}

	delay := cfg.Delay
	var output string
	var latestErr error

	total := time.Duration(cfg.Attempts-1) * delay
	ctx, cancel := context.WithTimeout(context.Background(), total)
	defer cancel()

	ticker := time.NewTicker(delay)
	defer ticker.Stop()

	for attempt := 1; attempt <= cfg.Attempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ticker.C:
				LogLevel("info", "Retrying command on node %s: %s\nAttempt %d/%d\n", ip, cmd, attempt, cfg.Attempts)
			case <-ctx.Done():
				return "", fmt.Errorf("retry timeout after %v: %w", total, ctx.Err())
			}
		}

		output, latestErr = RunCommandOnNode(cmd, ip)
		if latestErr == nil {
			return strings.TrimSpace(output), nil
		}

		if fatalSSHError(latestErr, cfg) || attempt == cfg.Attempts {
			break
		}

		delay = time.Duration(float64(delay) * cfg.DelayMultiplier)
		ticker.Reset(delay)
	}

	return "", fmt.Errorf("after %d attempts: %w", cfg.Attempts, latestErr)
}

// WaitForSSHReady waits for SSH to be ready on the node.
// Default max wait time: 3 mins. Retry 'SSH is ready' check every 10 seconds.
func WaitForSSHReady(ip string) error {
	return WaitForSSHReadyWithTimeout(ip, 3*time.Minute)
}

// WaitForSSHReadyWithTimeout is WaitForSSHReady with a caller-chosen window —
// fresh instances (provisioning/replacement) can take >5 min to authorize keys.
func WaitForSSHReadyWithTimeout(ip string, maxWait time.Duration) error {
	ticker := time.NewTicker(10 * time.Second)
	timeout := time.After(maxWait)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timed out waiting %s for SSH Ready on node ip %s", maxWait, ip)
		case <-ticker.C:
			cmdOutput, sshErr := RunCommandOnNode("ls -lrt", ip)
			if sshErr != nil {
				LogLevel("warn", "SSH Error: %s", sshErr)
				continue
			}
			if cmdOutput != "" {
				return nil
			}
		}
	}
}

// fatalSSHError checks if the error is "fatal" accordingly to the config passed and should not be retried.
func fatalSSHError(err error, cfg *RetryCfg) bool {
	msg := strings.ToLower(err.Error())

	for _, nonRetry := range cfg.NonRetryableErrorSubString {
		if strings.Contains(msg, nonRetry) {
			LogLevel("info", "Fatal error: %s, not retrying %s", msg, nonRetry)
			return true
		}
	}

	for _, retryMessage := range cfg.RetryableErrorSubString {
		if strings.Contains(msg, retryMessage) {
			LogLevel("info", "Retryable error: %s, retrying %s", msg, retryMessage)
			return false
		}
	}

	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		exit := exitErr.ExitStatus()
		for _, retryable := range cfg.RetryableExitCodes {
			if exit == retryable {
				LogLevel("info", "Retryable exit code: %d, retrying %d", exit, retryable)
				return false
			}
		}

		LogLevel("info", "Fatal exit code: %d, not retrying", exit)

		return true
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		LogLevel("info", "Context error: %s, retrying %s", msg, err)
		return false
	}

	return false
}

// configureSSHContext bounds both the TCP dial and the SSH handshake by ctx.
func configureSSHContext(ctx context.Context, host string) (*ssh.Client, error) {
	var (
		cfg *ssh.ClientConfig
		err error
	)

	sshUser, sshKeyPath := resolveSSHConfig()
	authMethod, err := publicKey(sshKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	cfg = &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			authMethod,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	var dialer net.Dialer
	netConn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, ReturnLogError("failed to dial: %w", err)
	}

	// The handshake has no context support, so a deadline on the raw conn bounds it.
	if deadline, ok := ctx.Deadline(); ok {
		_ = netConn.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = netConn.Close() })
	defer stop()

	sshConn, chans, reqs, err := ssh.NewClientConn(netConn, host, cfg)
	if err != nil {
		_ = netConn.Close()
		return nil, ReturnLogError("failed to dial: %w", err)
	}
	_ = netConn.SetDeadline(time.Time{})

	return ssh.NewClient(sshConn, chans, reqs), nil
}

func resolveSSHConfig() (user, path string) {
	var sshUser, sshKeyPath string
	infraProvisionerModule := os.Getenv("PROVISIONER_MODULE")

	switch infraProvisionerModule {
	case "legacy", "":
		sshKeyPath = os.Getenv("access_key")
		if sshKeyPath == "" && IsRunningInContainer() {
			sshKeyPath = "/go/src/github.com/rancher/distros-test-framework/config/.ssh/aws_key.pem"
		}
		sshUser = os.Getenv("aws_user")

		return sshUser, sshKeyPath
	case "qainfra":
		sshKeyPath = os.Getenv("SSH_LOCAL_KEY_PATH")
		sshUser = os.Getenv("SSH_USER")

		if IsRunningInContainer() {
			sshKeyPath = "/go/src/github.com/rancher/distros-test-framework/config/.ssh/aws_key.pem"
		}

		return sshUser, sshKeyPath
	default:
		LogLevel("warn", "Unknown PROVISIONER_MODULE: %s", infraProvisionerModule)
		return "", ""
	}
}

// RunCommandOnNodeWithStdin runs a command on the node streaming stdin over
// the established connection — no shell-encoded payloads or size limits.
func RunCommandOnNodeWithStdin(cmd, ip string, stdin io.Reader) (string, error) {
	conn, err := getOrDialSSH(ip + ":22")
	if err != nil {
		return "", fmt.Errorf("failed to connect to host %s: %w", ip, err)
	}

	session, err := conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdin = stdin
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	if runErr := session.Run(cmd); runErr != nil {
		return "", fmt.Errorf("command %s failed on %s: %w, stderr: %s",
			cmd, ip, runErr, stderrBuf.String())
	}

	return stdoutBuf.String(), nil
}

// RunCommandOnNodeContext runs cmd over SSH and closes the session when ctx ends. It returns
// only after the remote run has finished, so nothing keeps running after the caller moves on.
func RunCommandOnNodeContext(ctx context.Context, cmd, ip string) (string, error) {
	if cmd == "" {
		return "", ReturnLogError("cmd should not be empty")
	}

	// A dedicated client, never the pooled one: closing it on cancel must not abort other callers.
	conn, err := configureSSHContext(ctx, ip+":22")
	if err != nil {
		return "", fmt.Errorf("failed to connect to host %s: %w", ip, err)
	}
	defer conn.Close()

	session, err := newSessionContext(ctx, conn)
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	done := make(chan error, 1)
	go func() { done <- session.Run(cmd) }()

	select {
	case runErr := <-done:
		if runErr != nil {
			return "", fmt.Errorf("command %s failed on %s: %w, stderr: %s",
				cmd, ip, runErr, strings.TrimSpace(stderrBuf.String()))
		}

		return strings.TrimSpace(stdoutBuf.String()), nil
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
		// A blackholed peer never acks the session close, so close the client to unblock Run.
		_ = conn.Close()
		<-done

		return "", fmt.Errorf("command %s canceled on %s: %w", cmd, ip, ctx.Err())
	}
}

// newSessionContext closes conn when ctx ends, since NewSession cannot be canceled otherwise.
func newSessionContext(ctx context.Context, conn *ssh.Client) (*ssh.Session, error) {
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	session, err := conn.NewSession()
	if err != nil && ctx.Err() != nil {
		return nil, fmt.Errorf("%w: %w", err, ctx.Err())
	}

	return session, err
}

func runsshCommand(cmd string, conn *ssh.Client) (stdoutStr, stderrStr string, err error) {
	session, err := conn.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("failed to create session: %w\n", err)
	}
	defer session.Close()

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	errssh := session.Run(cmd)
	stdoutStr = stdoutBuf.String()
	stderrStr = stderrBuf.String()

	if errssh != nil {
		LogLevel("debug", "error from runsshCommand(): %v and stderror %s", errssh, stderrStr)
		return "", stderrStr, errssh
	}

	return stdoutStr, stderrStr, nil
}

// getOrDialSSH checks existence of a SSH connection or dials a new one with configureSSH(host).
func getOrDialSSH(host string) (*ssh.Client, error) {
	return getOrDialSSHContext(context.Background(), host)
}

// getOrDialSSHContext is getOrDialSSH with the health check and dial bounded by ctx.
func getOrDialSSHContext(ctx context.Context, host string) (*ssh.Client, error) {
	connPool.Lock()
	conn := connPool.connClient[host]
	connPool.Unlock()

	// if there is an existing connection, check if it's still valid.
	// if not, remove it from the pool.
	if conn != nil {
		err := pingSSH(ctx, conn)
		if err == nil {
			return conn, nil
		}
		_ = conn.Close()
		connPool.Lock()
		delete(connPool.connClient, host)
		connPool.Unlock()
	}

	// get a new connection and add it to the pool.
	newConn, err := configureSSHContext(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("failed to configure SSH: %w", err)
	}

	connPool.Lock()
	connPool.connClient[host] = newConn
	connPool.Unlock()

	LogLevel("debug", "SSH connection pool: %v\n", &connPool.connClient)

	return newConn, nil
}

// pingSSH closes conn when ctx ends, since a stuck session cannot be interrupted otherwise.
func pingSSH(ctx context.Context, conn *ssh.Client) error {
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	_, _, err := runsshCommand("echo ok", conn)

	return err
}

func publicKey(path string) (ssh.AuthMethod, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("unable to read private key: %v", err)
	}

	signer, signerErr := ssh.ParsePrivateKey(key)
	if signerErr != nil {
		return nil, fmt.Errorf("unable to parse private key: %v", signerErr)
	}

	return ssh.PublicKeys(signer), nil
}
