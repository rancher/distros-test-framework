package qainfra

import "strings"

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
