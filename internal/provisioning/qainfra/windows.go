package qainfra

import (
	"os"
	"regexp"
	"strings"
)

const windowsMarker = "# __WINDOWS_MODULE_ARGS__"

const windowsModuleArgs = `aws_ami_windows         = var.aws_ami_windows
  instance_type_windows   = var.instance_type_windows
  aws_volume_size_windows = var.aws_volume_size_windows
  aws_volume_type_windows = var.aws_volume_type_windows
  aws_windows_ssh_user    = var.aws_windows_ssh_user
  windows_enable_rdp      = var.windows_enable_rdp`

// injectWindowsModuleArgs adds Windows inputs only when requested, keeping Linux runs
// valid on qa-infra refs that predate them.
func injectWindowsModuleArgs(mainTf string, windows bool) string {
	args := ""
	if windows {
		args = windowsModuleArgs
	}

	return strings.ReplaceAll(mainTf, windowsMarker, args)
}

var windowsNodeRE = regexp.MustCompile(`(?i)"?\bos"?\s*[=:]\s*"windows"`)

// windowsRequested checks the effective topology: the env-built nodes override wins,
// otherwise the vars.tfvars nodes list (e.g. from the Jenkins TFVARS blob).
func windowsRequested(tfvarsPath string) (bool, error) {
	winWorkers, err := envInt("no_of_windows_worker_nodes", 0)
	if err != nil {
		return false, err
	}
	if winWorkers > 0 {
		return true, nil
	}
	// A non-empty env topology replaces the tfvars nodes, so tfvars Windows entries no longer apply.
	if nodesJSON, nodesErr := buildNodesTFVar(); nodesErr == nil && nodesJSON != "" {
		return false, nil
	}

	content, err := os.ReadFile(tfvarsPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return windowsNodeRE.Match(content), nil
}
