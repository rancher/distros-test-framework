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

	"golang.org/x/crypto/ssh"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/logger"
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
			return c.Stderr.(*bytes.Buffer).String(), err
		}
	}

	return output.String(), nil
}

// RunCommandOnNode executes a command on the node SSH
func RunCommandOnNode(cmd, ip string) (string, error) {
	if cmd == "" {
		return "", ReturnLogError("cmd should not be empty")
	}
	LogLevel("debug", "Execute: %s on %s", cmd, ip)

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

	if cleanedStderr != "" && (!strings.Contains(stderr, "exited") || !strings.Contains(cleanedStderr, "1") ||
		!strings.Contains(cleanedStderr, "2")) {
		return cleanedStderr, nil
	} else if cleanedStderr != "" {
		return "", fmt.Errorf("command: %s failed with error: %v\n", cmd, stderr)
	}

	return stdout, err
}

// BasePath returns the base path of the project.
func BasePath() string {
	_, callerFilePath, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(callerFilePath), "..")
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
func PrintBase64Encoded(path string) error {
	file, err := os.ReadFile(path)
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
	cluster := factory.ClusterConfig()

	authMethod, err := publicKey(cluster.AwsConfig.AccessKey)
	if err != nil {
		return nil, ReturnLogError("failed to get public key: %v", err)
	}
	cfg = &ssh.ClientConfig{
		User: cluster.AwsConfig.AwsUser,
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

func runsshCommand(cmd string, conn *ssh.Client) (stdoutStr, stderrStr string, err error) {
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
	stdoutStr = stdoutBuf.String()
	stderrStr = stderrBuf.String()

	if errssh != nil {
		LogLevel("warn", "%v\n", stderrStr)
		return "", stderrStr, errssh
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
func GetJournalLogs(level, ip string) string {
	if level == "" {
		LogLevel("warn", "level should not be empty")
		return ""
	}

	levels := map[string]bool{"info": true, "debug": true, "warn": true, "error": true, "fatal": true}
	if _, ok := levels[level]; !ok {
		LogLevel("warn", "Invalid log level: %s\n", level)
		return ""
	}

	product, err := Product()
	if err != nil {
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

// ReturnLogError logs the error and returns it.
func ReturnLogError(format string, args ...interface{}) error {
	log := logger.AddLogger()
	err := formatLogArgs(format, args...)

	if err != nil {
		pc, file, line, ok := runtime.Caller(1)
		if ok {
			funcName := runtime.FuncForPC(pc).Name()

			formattedPath := fmt.Sprintf("file:%s:%d", file, line)
			log.Error(fmt.Sprintf("%s\nLast call: %s in %s", err.Error(), funcName, formattedPath))
		} else {
			log.Error(err.Error())
		}
	}

	return err
}

// LogLevel logs the message with the specified level.
func LogLevel(level, format string, args ...interface{}) {
	log := logger.AddLogger()
	msg := formatLogArgs(format, args...)

	switch level {
	case "debug":
		log.Debug(msg)
	case "info":
		log.Info(msg)
	case "warn":
		log.Warn(msg)
	case "error":
		pc, file, line, ok := runtime.Caller(1)
		if ok {
			funcName := runtime.FuncForPC(pc).Name()
			log.Error(fmt.Sprintf("%s\nLast call: %s in %s:%d", msg, funcName, file, line))
		}
		log.Error(msg)
	case "fatal":
		pc, file, line, ok := runtime.Caller(1)
		if ok {
			funcName := runtime.FuncForPC(pc).Name()
			log.Fatal(fmt.Sprintf("%s\nLast call: %s in %s:%d", msg, funcName, file, line))
		}
		log.Fatal(msg)
	default:
		log.Info(msg)
	}
}

// formatLogArgs formats the logger message.
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

func UninstallProduct(product, nodeType, ip string) error {
	var scriptName string
	paths := []string{
		"/usr/local/bin",
		"/opt/local/bin",
		"/usr/bin",
		"/usr/sbin",
		"/usr/local/sbin",
		"/bin",
		"/sbin",
	}

	switch product {
	case "k3s":
		if nodeType == "agent" {
			scriptName = "k3s-agent-uninstall.sh"
		} else {
			scriptName = "k3s-uninstall.sh"
		}
	case "rke2":
		scriptName = "rke2-uninstall.sh"
	default:
		return fmt.Errorf("unsupported product: %s", product)
	}

	foundPath, err := findScriptPath(paths, scriptName, ip)
	if err != nil {
		return fmt.Errorf("failed to find uninstall script for %s: %v", product, err)
	}

	pathName := fmt.Sprintf("%s-uninstall.sh", product)
	if product == "k3s" && nodeType == "agent" {
		pathName = "k3s-agent-uninstall.sh"
	}

	uninstallCmd := fmt.Sprintf("sudo %s/%s", foundPath, pathName)
	_, err = RunCommandOnNode(uninstallCmd, ip)

	return err
}

func findScriptPath(paths []string, pathName, ip string) (string, error) {
	for _, path := range paths {
		checkCmd := fmt.Sprintf("if [ -f %s/%s ]; then echo 'found'; else echo 'not found'; fi",
			path, pathName)
		output, err := RunCommandOnNode(checkCmd, ip)
		if err != nil {
			return "", err
		}
		output = strings.TrimSpace(output)
		if output == "found" {
			return path, nil
		}
	}

	searchPath := fmt.Sprintf("find / -name %s 2>/dev/null", pathName)
	fullPath, err := RunCommandOnNode(searchPath, ip)
	if err != nil {
		return "", err
	}

	fullPath = strings.TrimSpace(fullPath)
	if fullPath == "" {
		return "", fmt.Errorf("script %s not found", pathName)
	}

	return filepath.Dir(fullPath), nil
}

// MatchWithPath verify expected files found in the actual file list
func MatchWithPath(actualFileList, expectedFileList []string) error {
	for i := 0; i < len(expectedFileList); i++ {
		if !stringInSlice(expectedFileList[i], actualFileList) {
			return ReturnLogError(fmt.Sprintf("FAIL: Expected file: %s NOT found in actual list",
				expectedFileList[i]))
		}
		LogLevel("info", "PASS: Expected file %s found", expectedFileList[i])
	}

	for i := 0; i < len(actualFileList); i++ {
		if !stringInSlice(actualFileList[i], expectedFileList) {
			LogLevel("info", "Actual file %s found as well which was not in the expected list",
				actualFileList[i])
		}
	}

	return nil
}

// stringInSlice verify if a string is found in the list of strings
func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// appendNodeIfMissing appends a value to a slice if that value does not already exist in the slice.
func appendNodeIfMissing(slice []Node, i Node) []Node {
	for _, ele := range slice {
		if ele == i {
			return slice
		}
	}
	return append(slice, i)
}
func EncloseSqBraces(ip string) string {
	return "[" + ip + "]"
}
