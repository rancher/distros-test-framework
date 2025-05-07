package shared

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	LogLevel("info", "Running command on node with ssh error retry %s: %s\ncfg: %+v\n", ip, cmd, cfg)

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

func configureSSH(host string) (*ssh.Client, error) {
	var (
		cfg *ssh.ClientConfig
		err error
	)

	// get access key and user from cluster config.
	kubeConfig := os.Getenv("KUBE_CONFIG")
	if kubeConfig == "" {
		productCfg := AddProductCfg()
		cluster = ClusterConfig(productCfg)
	} else {
		cluster, err = addClusterFromKubeConfig(nil)
		if err != nil {
			return nil, ReturnLogError("failed to get cluster from kubeconfig: %w", err)
		}
	}

	authMethod, err := publicKey(cluster.Aws.AccessKey)
	if err != nil {
		return nil, ReturnLogError("failed to get public key: %w", err)
	}

	cfg = &ssh.ClientConfig{
		User: cluster.Aws.AwsUser,
		Auth: []ssh.AuthMethod{
			authMethod,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", host, cfg)
	if err != nil {
		return nil, ReturnLogError("failed to dial: %w", err)
	}

	return conn, nil
}

func runsshCommand(cmd string, conn *ssh.Client) (stdoutStr, stderrStr string, err error) {
	session, err := conn.NewSession()
	if err != nil {
		return "", "", ReturnLogError("failed to create session: %w\n", err)
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
		LogLevel("warn", "%v\n", stderrStr)
		return "", stderrStr, errssh
	}

	return stdoutStr, stderrStr, nil
}

// getOrDial checks existence of a SSH connection or dials a new one with configureSSH(host).
func getOrDial(host string) (*ssh.Client, error) {
	connPool.Lock()
	conn := connPool.connClient[host]
	connPool.Unlock()

	// if there is an existing connection, check if it's still valid.
	// if not, remove it from the pool.
	if conn != nil {
		_, _, err := runsshCommand("echo ok", conn)
		if err == nil {
			return conn, nil
		}
		_ = conn.Close()
		connPool.Lock()
		delete(connPool.connClient, host)
		connPool.Unlock()
	}

	// get a new connection and add it to the pool.
	newConn, err := configureSSH(host)
	if err != nil {
		return nil, fmt.Errorf("failed to configure SSH: %v", err)
	}

	connPool.Lock()
	connPool.connClient[host] = newConn
	connPool.Unlock()

	LogLevel("info", "SSH connection pool: %v\n", &connPool.connClient)

	return newConn, nil
}
