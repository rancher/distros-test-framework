package resources

import (
	"fmt"
	"strings"
)

// RunCommandOnPrivateNode runs cmd on a node without public IP from this host,
// tunneling through the bastion with ProxyCommand; the key never leaves the controller.
func RunCommandOnPrivateNode(cmd, nodeIP, bastionIP, user, keyPath string) (string, error) {
	common := []string{
		"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		"-o", "IdentitiesOnly=yes", "-o", "ConnectTimeout=30", "-i", keyPath,
	}
	proxy := "ssh " + strings.Join(common, " ") + fmt.Sprintf(" -W %%h:%%p %s@%s", user, bastionIP)
	args := make([]string, 0, len(common)+6)
	args = append(args, common...)
	args = append(args, "-o", "ProxyCommand="+proxy, "-o", "LogLevel=ERROR", user+"@"+nodeIP, cmd)

	out, err := RunHostArgs("ssh", args...)
	if err != nil {
		return out, fmt.Errorf("ssh %s via bastion %s: %w", nodeIP, bastionIP, err)
	}

	return out, nil
}
