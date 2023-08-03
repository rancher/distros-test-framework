package shared

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rancher/distros-test-framework/config"
	"golang.org/x/crypto/ssh"
)

// RunCommandHost executes a command on the host
func RunCommandHost(cmds ...string) (string, error) {
	if cmds == nil {
		return "", fmt.Errorf("cmd should not be empty")
	}

	var output, errOut bytes.Buffer
	for _, cmd := range cmds {
		c := exec.Command("bash", "-c", cmd)
		c.Stdout = &output
		c.Stderr = &errOut

		err := c.Run()
		if err != nil {
			fmt.Println(c.Stderr.(*bytes.Buffer).String())
			return output.String(), fmt.Errorf("executing command: %s: %w", cmd, err)
		}

		if errOut.Len() > 0 {
			fmt.Println("returning Stderr if not null, this might not be an error",
				errOut.String())
		}
	}

	return output.String(), nil
}

// RunCommandOnNode executes a command on the node SSH
func RunCommandOnNode(cmd, serverIP string) (string, error) {
	if cmd == "" {
		return "", fmt.Errorf("cmd should not be empty")
	}

	host := serverIP + ":22"
	conn, err := configureSSH(host)
	if err != nil {
		return fmt.Errorf("failed to configure SSH: %v", err).Error(), err
	}

	stdout, stderr, err := runsshCommand(cmd, conn)
	if err != nil {
		return "", fmt.Errorf(
			"command: %s \n failed on run ssh : %s with error %w",
			cmd,
			serverIP,
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
		return fmt.Errorf("\ncommand: %s \n failed with error: %v", cmd, stderr).Error(), err
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
			return err
		}
		fmt.Println(string(content) + "\n")
	}

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
func GetVersion(command string) string {
	ips := FetchNodeExternalIP()
	for _, ip := range ips {
		res, err := RunCommandOnNode(command, ip)
		if err != nil {
			return err.Error()
		}
		return res
	}
	return ""
}

// GetProduct returns the distro product based on the config file
func GetProduct() (string, error) {
	cfg, err := config.LoadConfigEnv("./config")
	if err != nil {
		return "", err
	}
	if cfg.Product != "k3s" && cfg.Product != "rke2" {
		return "", errors.New("unknown product")
	}
	return cfg.Product, nil
}

// GetProductVersion return the version for a specific distro product
func GetProductVersion(product string) (string, error) {
	if product != "rke2" && product != "k3s" {
		return "", fmt.Errorf("unsupported product: %s", product)
	}
	version := GetVersion(product + " -v")
	if version == "" {
		return "", fmt.Errorf("failed to get version for product: %s", product)
	}

	return version, nil
}

// AddHelmRepo adds a helm repo to the cluster.
func AddHelmRepo(name, url string) (string, error) {
	InstallHelm := "curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash"
	addRepo := fmt.Sprintf("helm repo add %s %s", name, url)
	update := "helm repo update"
	installRepo := fmt.Sprintf("helm install %s %s/%s -n kube-system --kubeconfig=%s",
		name, name, name, KubeConfigFile)

	nodeExternalIP := FetchNodeExternalIP()
	for _, ip := range nodeExternalIP {
		_, err := RunCommandOnNode(InstallHelm, ip)
		if err != nil {
			return "", err
		}
	}

	return RunCommandHost(addRepo, update, installRepo)
}

func publicKey(path string) (ssh.AuthMethod, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	return ssh.PublicKeys(signer), nil
}

func configureSSH(host string) (*ssh.Client, error) {
	var cfg *ssh.ClientConfig

	authMethod, err := publicKey(AccessKey)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	return conn, nil
}

func runsshCommand(cmd string, conn *ssh.Client) (string, string, error) {
	session, err := conn.NewSession()
	if err != nil {
		return "", "", err
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
		return stdoutStr, stderrStr, fmt.Errorf("error on command execution: %v", errssh)
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
