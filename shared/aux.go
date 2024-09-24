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

	"github.com/rancher/distros-test-framework/pkg/logger"
)

var log = logger.AddLogger()

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
			return c.Stderr.(*bytes.Buffer).String(), err
		}
	}

	return output.String(), nil
}

// RunCommandOnNode executes a command on the node SSH.
func RunCommandOnNode(cmd, ip string) (string, error) {
	if cmd == "" {
		return "", ReturnLogError("cmd should not be empty")
	}

	host := ip + ":22"
	conn, err := configureSSH(host)
	if err != nil {
		return "", ReturnLogError("failed to configure SSH: %w\n", err)
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
			return ReturnLogError("failed to read file: %w\n", err)
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
	for i := range pods {
		if strings.Contains(pods[i].Name, str) {
			count++
		}
	}

	return count
}

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
			c.AwsEc2.AccessKey,
			localPath,
			c.AwsEc2.AwsUser,
			remotePath,
		)

		res, cmdErr := RunCommandHost(scp)
		if res != "" {
			LogLevel("warn", "scp output: %s\n", res)
		}
		if cmdErr != nil {
			LogLevel("error", "failed to run scp: %v\n", cmdErr)
			return cmdErr
		}

		chmod := "sudo chmod +wx " + remotePath
		_, cmdErr = RunCommandOnNode(chmod, ip)
		if cmdErr != nil {
			LogLevel("error", "failed to run chmod: %v\n", cmdErr)
			return cmdErr
		}
	}

	return nil
}

// CheckHelmRepo checks a helm chart is available on the repo.
func CheckHelmRepo(name, url, version string) (string, error) {
	addRepo := fmt.Sprintf("helm repo add %s %s", name, url)
	update := "helm repo update"
	searchRepo := fmt.Sprintf("helm search repo %s --devel -l | grep %s", name, version)

	return RunCommandHost(addRepo, update, searchRepo)
}

func publicKey(path string) (ssh.AuthMethod, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, ReturnLogError("failed to read private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, ReturnLogError("failed to parse private key: %w", err)
	}

	return ssh.PublicKeys(signer), nil
}

func configureSSH(host string) (*ssh.Client, error) {
	var (
		cfg *ssh.ClientConfig
		err error
	)

	// get access key and user from cluster config.
	kubeConfig := os.Getenv("KUBE_CONFIG")
	if kubeConfig == "" {
		cluster = ClusterConfig()
	} else {
		cluster, err = addClusterFromKubeConfig(nil)
		if err != nil {
			return nil, ReturnLogError("failed to get cluster from kubeconfig: %w", err)
		}
	}

	authMethod, err := publicKey(cluster.AwsEc2.AccessKey)
	if err != nil {
		return nil, ReturnLogError("failed to get public key: %w", err)
	}

	cfg = &ssh.ClientConfig{
		User: cluster.AwsEc2.AwsUser,
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

// JoinCommands joins the first command with some arg.
//
// That could separators like ";" , | , "&&" etc.
//
// Example:
// "kubectl get nodes -o wide : | grep IMAGES" =>
//
// "kubectl get nodes -o wide --kubeconfig /tmp/kubeconfig | grep IMAGES".
func JoinCommands(cmd, kubeconfigFlag string) string {
	cmds := strings.Split(cmd, ":")
	joinedCmd := cmds[0] + kubeconfigFlag

	if len(cmds) > 1 {
		secondCmd := strings.Join(cmds[1:], ",")
		joinedCmd += " " + secondCmd
	}

	return joinedCmd
}

// GetJournalLogs returns the journal logs for a specific product.
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

	product, _, err := Product()
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
	msg := formatLogArgs(format, args...)

	envLogLevel := os.Getenv("LOG_LEVEL")
	envLogLevel = strings.ToLower(envLogLevel)

	switch level {
	case "debug":
		if envLogLevel == "debug" {
			log.Debug(msg)
		} else {
			return
		}
	case "info":
		if envLogLevel == "info" || envLogLevel == "" || envLogLevel == "debug" {
			log.Info(msg)
		} else {
			return
		}
	case "warn":
		if envLogLevel == "warn" || envLogLevel == "" || envLogLevel == "info" || envLogLevel == "debug" {
			log.Warn(msg)
		} else {
			return
		}
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
		return fmt.Errorf("%s", format)
	}
	if e, ok := args[0].(error); ok {
		if len(args) > 1 {
			return fmt.Errorf(format, args[1:]...)
		}

		return e
	}

	return fmt.Errorf(format, args...)
}

// fileExists Checks if a file exists in a directory.
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

	foundPath, findErr := checkFiles(product, paths, scriptName, ip)
	if findErr != nil {
		return findErr
	}

	pathName := product + "-uninstall.sh"
	if product == "k3s" && nodeType == "agent" {
		pathName = "k3s-agent-uninstall.sh"
	}

	uninstallCmd := fmt.Sprintf("sudo %s/%s", foundPath, pathName)
	_, err := RunCommandOnNode(uninstallCmd, ip)

	return err
}

func checkFiles(product string, paths []string, scriptName, ip string) (string, error) {
	var foundPath string
	for _, path := range paths {
		checkCmd := fmt.Sprintf("if [ -f %s/%s ]; then echo 'found'; else echo 'not found'; fi",
			path, scriptName)
		output, err := RunCommandOnNode(checkCmd, ip)
		if err != nil {
			return "", err
		}
		output = strings.TrimSpace(output)
		if output == "found" {
			foundPath = path
		} else {
			foundPath, err = FindPath(scriptName, ip)
			if err != nil {
				return "", fmt.Errorf("failed to find uninstall script for %s: %w", product, err)
			}
		}
	}

	return foundPath, nil
}

func FindPath(name, ip string) (string, error) {
	searchPath := fmt.Sprintf("find / -type f -executable -name %s 2>/dev/null | grep -v data | sed 1q", name)
	fullPath, err := RunCommandOnNode(searchPath, ip)
	if err != nil {
		return "", err
	}

	if fullPath == "" {
		return "", fmt.Errorf("path for %s not found", name)
	}

	return strings.TrimSpace(fullPath), nil
}

// MatchWithPath verify expected files found in the actual file list.
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

// ReplaceFileContents reads file from path and replaces them based on key value pair provided.
func ReplaceFileContents(filePath string, replaceKV map[string]string) error {
	contents, err := os.ReadFile(filePath)
	if err != nil {
		return ReturnLogError("File does not exist: %v", filePath)
	}

	for key, value := range replaceKV {
		if strings.Contains(string(contents), key) {
			contents = bytes.ReplaceAll(contents, []byte(key), []byte(value))
		}
	}

	err = os.WriteFile(filePath, contents, 0o666)
	if err != nil {
		return ReturnLogError("Write to File failed: %v", filePath)
	}

	return nil
}

// stringInSlice verify if a string is found in the list of strings.
func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}

	return false
}

// appendNodeIfMissing appends a value to a slice if that value does not already exist in the slice.
func appendNodeIfMissing(slice []Node, i *Node) []Node {
	for _, ele := range slice {
		if ele == *i {
			return slice
		}
	}

	return append(slice, *i)
}

func EncloseSqBraces(ip string) string {
	return "[" + ip + "]"
}

// CleanString removes spaces and new lines from a string.
func CleanString(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "\n", ""), " ", "")
}

// CleanSliceStrings removes spaces and new lines from a slice of strings.
func CleanSliceStrings(stringsSlice []string) []string {
	for i, str := range stringsSlice {
		stringsSlice[i] = CleanString(str)
	}

	return stringsSlice
}
