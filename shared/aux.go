package shared

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/log"
	"golang.org/x/crypto/ssh"
)

// RunCommandHost executes a command on the host
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
			LogLevel("warn", "\nsomething happening on command:\n %s: \nwith error: %s\n", cmd,
				c.Stderr.(*bytes.Buffer).String())
			return "", err
		}
	}

	return output.String(), nil
}

// RunCommandOnNode executes a command on the node SSH
func RunCommandOnNode(cmd, ip string) (string, error) {
	if cmd == "" {
		return "", ReturnLogError("cmd should not be empty")
	}

	host := ip + ":22"
	conn, err := configureSSH(host)
	if err != nil {
		return "", ReturnLogError("failed to configure SSH: %v\n", err)
	}
	stdout, stderr, err := runsshCommand(cmd, conn)
	if err != nil && !strings.Contains(stderr, "restart") {
		return "", fmt.Errorf(
			"command: %s failed on run ssh: %s with error: %w\n",
			cmd,
			ip,
			err,
		)
	}

	stdout = strings.TrimSpace(stdout)
	stderr = strings.TrimSpace(stderr)

	cleanedStderr := strings.ReplaceAll(stderr, "\n", "")
	cleanedStderr = strings.ReplaceAll(cleanedStderr, "\t", "")

	if cleanedStderr != "" && (!strings.Contains(stderr, "exited") ||
		!strings.Contains(cleanedStderr, "1") ||
		!strings.Contains(cleanedStderr, "2")) {
		return cleanedStderr, nil
	} else if cleanedStderr != "" {
		return "", fmt.Errorf("command: %s failed with error: %v\n", cmd, stderr)
	}

	return stdout, err
}

// BasePath returns the base path of the project.
func BasePath() string {
	_, b, _, _ := runtime.Caller(0)

	return filepath.Join(filepath.Dir(b), "../..")
}

// PrintFileContents prints the contents of the file as [] string.
func PrintFileContents(f ...string) error {
	for _, file := range f {
		content, err := os.ReadFile(file)
		if err != nil {
			return ReturnLogError("failed to read file: %v\n", err)
		}
		fmt.Println(string(content) + "\n")
	}

	return nil
}

// PrintBase64Encoded prints the base64 encoded contents of the file as string.
func PrintBase64Encoded(filepath string) error {
	file, err := os.ReadFile(filepath)
	if err != nil {
		return ReturnLogError("failed to encode file %s: %w", file, err)
	}

	encoded := base64.StdEncoding.EncodeToString(file)
	fmt.Println(encoded)

	return nil
}

// CountOfStringInSlice Used to count the pods using prefix passed in the list of pods.
func CountOfStringInSlice(str string, pods []Pod) int {
	var count int
	for _, p := range pods {
		if strings.Contains(p.Name, str) {
			count++
		}
	}

	return count
}

// GetVersion returns the rke2 or k3s version
func GetVersion(cmd string) (string, error) {
	var res string
	var err error
	ips := FetchNodeExternalIP()
	for _, ip := range ips {
		res, err = RunCommandOnNode(cmd, ip)
		if err != nil {
			return "", ReturnLogError("failed to run command on node: %v\n", err)
		}
	}

	return res, nil
}

// GetProduct returns the distro product based on the config file
func GetProduct() (string, error) {
	cfg, err := config.AddConfigEnv("./config")
	if err != nil {
		return "", ReturnLogError("failed to get config: %v\n", err)
	}
	if cfg.Product != "k3s" && cfg.Product != "rke2" {
		return "", ReturnLogError("unknown product")
	}

	return cfg.Product, nil
}

// GetProductVersion return the version for a specific distro product
func GetProductVersion(product string) (string, error) {
	if product != "rke2" && product != "k3s" {
		return "", ReturnLogError("unsupported product: %s\n", product)
	}
	version, err := GetVersion(product + " -v")
	if err != nil {
		return "", ReturnLogError("failed to get version for product: %s, error: %v\n", product, err)
	}

	return version, nil
}

// AddHelmRepo adds a helm repo to the cluster.
func AddHelmRepo(name, url string) (string, error) {
	installHelm := "curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash"
	addRepo := fmt.Sprintf("helm repo add %s %s", name, url)
	update := "helm repo update"
	installRepo := fmt.Sprintf("helm install %s %s/%s -n kube-system --kubeconfig=%s",
		name, name, name, KubeConfigFile)

	nodeExternalIP := FetchNodeExternalIP()
	for _, ip := range nodeExternalIP {
		_, err := RunCommandOnNode(installHelm, ip)
		if err != nil {
			return "", ReturnLogError("failed to install helm: %v", err)
		}
	}

	return RunCommandHost(addRepo, update, installRepo)
}

func publicKey(path string) (ssh.AuthMethod, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, ReturnLogError("failed to read private key: %v", err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, ReturnLogError("failed to parse private key: %v", err)
	}

	return ssh.PublicKeys(signer), nil
}

func configureSSH(host string) (*ssh.Client, error) {
	var cfg *ssh.ClientConfig

	authMethod, err := publicKey(AccessKey)
	if err != nil {
		return nil, ReturnLogError("failed to get public key: %v", err)
	}
	cfg = &ssh.ClientConfig{
		User: AwsUser,
		Auth: []ssh.AuthMethod{
			authMethod,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	conn, err := ssh.Dial("tcp", host, cfg)
	if err != nil {
		return nil, ReturnLogError("failed to dial: %v", err)
	}

	return conn, nil
}

func runsshCommand(cmd string, conn *ssh.Client) (string, string, error) {
	session, err := conn.NewSession()
	if err != nil {
		return "", "", ReturnLogError("failed to create session: %v\n", err)
	}
	defer session.Close()

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	errssh := session.Run(cmd)
	stdoutStr := stdoutBuf.String()
	stderrStr := stderrBuf.String()

	if errssh != nil {
		return stdoutStr, stderrStr, errssh
	}

	return stdoutStr, stderrStr, nil
}

// JoinCommands joins the first command with some arg
func JoinCommands(cmd, kubeconfigFlag string) string {
	cmds := strings.Split(cmd, ":")
	joinedCmd := cmds[0] + kubeconfigFlag

	if len(cmds) > 1 {
		secondCmd := strings.Join(cmds[1:], ",")
		joinedCmd += " " + secondCmd
	}

	return joinedCmd
}

// GetJournalLogs returns the journal logs for a specific product
func GetJournalLogs(product, ip string) (string, error) {
	cmd := fmt.Sprintf("journalctl -u %s* --no-pager", product)
	return RunCommandOnNode(cmd, ip)
}

// ReturnLogError logs the error and returns it.
func ReturnLogError(format string, args ...interface{}) error {
	logger := log.AddLogger(false)
	err := formatLogArgs(format, args...)

	pc, file, line, ok := runtime.Caller(1)
	if ok {
		funcName := runtime.FuncForPC(pc).Name()
		logger.Error(fmt.Sprintf("%s\nLast call: %s in %s:%d", err.Error(), funcName, file, line))
	} else {
		logger.Error(err.Error())
	}

	return err
}

// LogLevel logs the message with the specified level.
func LogLevel(level, format string, args ...interface{}) {
	logger := log.AddLogger(false)
	msg := formatLogArgs(format, args...)

	switch level {
	case "debug":
		logger.Debug(msg)
	case "info":
		logger.Info(msg)
	case "warn":
		logger.Warn(msg)
	case "error":
		logger.Error(msg)
	}
}

// formatLogArgs formats the log message.
func formatLogArgs(format string, args ...interface{}) error {
	if len(args) == 0 {
		return fmt.Errorf(format)
	}
	if e, ok := args[0].(error); ok {
		if len(args) > 1 {
			return fmt.Errorf(format, args[1:]...)
		}
		return e
	}

	return fmt.Errorf(format, args...)
}

// fileExists Checks if a file exists in a directory
func fileExists(files []os.DirEntry, workload string) bool {
	for _, file := range files {
		if file.Name() == workload {
			return true
		}
	}

	return false
}
