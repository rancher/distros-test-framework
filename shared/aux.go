package shared

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
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
			c.Aws.AccessKey,
			localPath,
			c.Aws.AwsUser,
			remotePath,
		)

		LogLevel("debug", "running scp command: %s\n", scp)
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

func FindPath(name, ip string) (string, error) {
	if ip == "" {
		return "", errors.New("ip should not be empty")
	}

	if name == "" {
		return "", errors.New("name should not be empty")
	}

	// adding common paths to the environment variable PATH since in some os's not all paths are available.
	commonPaths := "/var/lib/rancher/rke2/bin:" +
		"/var/rancher/rke2/bin:" +
		"/var/lib/rancher:" +
		"/var/rancher:" +
		"/opt/rke2/bin:" +
		"/var/lib/rancher/k3s/bin:" +
		"/var/rancher/k3s/bin:" +
		"/opt/k3s/bin:" +
		"/usr/local/bin:" +
		"/usr/bin:"

	// adding the common paths to the PATH environment variable by sourcing it from a file.
	envFile := "find_path_env.sh"
	err := ExportEnvProfileNode([]string{ip}, map[string]string{"PATH": commonPaths}, envFile)
	if err != nil {
		return "", fmt.Errorf("failed to create environment file: %w", err)
	}

	sourcedCmds := []string{
		fmt.Sprintf(". /etc/profile.d/%s && which %s 2>/dev/null", envFile, name),
		fmt.Sprintf(". /etc/profile.d/%s && command -v %s 2>/dev/null", envFile, name),
		fmt.Sprintf(". /etc/profile.d/%s && type -p %s 2>/dev/null", envFile, name),
	}

	for _, cmd := range sourcedCmds {
		path, err := RunCommandOnNode(cmd, ip)
		if err != nil {
			LogLevel("warn", "Failed to find %s with sourced environment: %v", name, err)
			continue
		}
		if path != "" {
			return strings.TrimSpace(path), nil
		}
	}

	findCmd := fmt.Sprintf("find / -type f -executable -name %s 2>/dev/null | "+
		" grep -v data | sed 1q", name)
	fullPath, err := RunCommandOnNode(findCmd, ip)
	if err != nil {
		return "", fmt.Errorf("failed to run command %s: %w", findCmd, err)
	}
	if fullPath == "" {
		return "", fmt.Errorf("path for %s not found", name)
	}

	return strings.TrimSpace(fullPath), nil
}

// MatchWithPath verify expected files found in the actual file list.
func MatchWithPath(actualFileList, expectedFileList []string) error {
	for i := 0; i < len(expectedFileList); i++ {
		if !slices.Contains(actualFileList, expectedFileList[i]) {
			return ReturnLogError("FAIL: Expected file: %s NOT found in actual list",
				expectedFileList[i])
		}
		LogLevel("info", "PASS: Expected file %s found", expectedFileList[i])
	}

	for i := 0; i < len(actualFileList); i++ {
		if !slices.Contains(expectedFileList, actualFileList[i]) {
			LogLevel("info", "Actual file %s found as well which was not in the expected list",
				actualFileList[i])
		}
	}

	return nil
}

// CopyFileContents reads file from path and copies them locally.
func CopyFileContents(srcPath, destPath string) error {
	contents, err := os.ReadFile(srcPath)
	if err != nil {
		return ReturnLogError("File does not exist: %v", srcPath)
	}

	err = os.WriteFile(destPath, contents, 0o666)
	if err != nil {
		return ReturnLogError("Write to File failed: %v", destPath)
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

// SliceContainsString verify if a string is found in the list of strings.
func SliceContainsString(list []string, a string) bool {
	for _, b := range list {
		if strings.Contains(a, b) {
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

// SystemCtlCmd returns the systemctl command based on the action and service.
// it can be used alone to create the command and be ran by another function caller.
//
// Action can be: start | stop | restart | status | enable.
func SystemCtlCmd(service, action string) (string, error) {
	systemctlCmdMap := map[string]string{
		"stop":    "sudo systemctl --no-block stop",
		"start":   "sudo systemctl --no-block start",
		"restart": "sudo systemctl --no-block restart",
		"status":  "sudo systemctl --no-block status",
		"enable":  "sudo systemctl --no-block enable",
	}

	sysctlPrefix, ok := systemctlCmdMap[action]
	if !ok {
		return "", ReturnLogError("action value should be: start | stop | restart | status | enable")
	}

	return fmt.Sprintf("%s %s", sysctlPrefix, service), nil
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
