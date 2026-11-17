package qainfra

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// Dual-stack and IPv6-only runs use qa-infra's tofu/aws/modules/<env> and
// ansible/<product>/<env> instead of cluster_nodes and ansible/<product>/default.
const (
	dualstackModule = "dualstack"
	ipv6OnlyModule  = "ipv6only"
	envDualstack    = "dualstack"
	envIPv6         = "ipv6"
)

// The dualstack/ipv6 modules do not declare these cluster_nodes inputs.
var clusterNodesOnlyArgsRE = regexp.MustCompile(`(?m)^[ \t]*(airgap_setup|proxy_setup)[ \t]*=[^\r\n]*(\r?\n|$)`)

// networkEnv maps ENV_MODULE onto the qa-infra ENV name, or "" for other modules.
func networkEnv(module string) string {
	switch strings.ToLower(strings.TrimSpace(module)) {
	case dualstackModule:
		return envDualstack
	case ipv6OnlyModule:
		return envIPv6
	default:
		return ""
	}
}

// injectNetworkModuleArgs swaps cluster_nodes-only inputs for the dualstack/ipv6 module inputs.
func injectNetworkModuleArgs(mainTf, env string) string {
	if env == "" {
		return mainTf
	}
	mainTf = clusterNodesOnlyArgsRE.ReplaceAllString(mainTf, "")
	args := fmt.Sprintf("enable_ipv6         = true\n"+
		"  enable_public_ip    = %t\n"+
		"  no_of_bastion_nodes = 1\n"+
		"  aws_bastion_subnet  = var.aws_bastion_subnet != \"\" ? var.aws_bastion_subnet : var.aws_subnet\n"+
		"  kube_api_host_ipv6  = var.kube_api_host_ipv6",
		env == envDualstack)

	return strings.ReplaceAll(mainTf, airgapModuleMarker, args)
}

// buildNetworkInventory writes bastion + node groups with ansible_host_ipv6;
// IPv6-only nodes are reached through the bastion, dual-stack nodes directly.
func buildNetworkInventory(data *clusterNodesJSON, env, sshUser, keyPath string) (string, error) {
	if data.Bastion == nil || data.Bastion.PublicIP == "" {
		return "", fmt.Errorf("%s inventory needs a bastion with a public IP in cluster_nodes_json", env)
	}
	assigned := assignNodeGroups(data, "")

	var b strings.Builder
	writeNetworkVars(&b, data, sshUser)
	if err := writeNetworkHosts(&b, data, env, sshUser, keyPath, assigned); err != nil {
		return "", err
	}
	writeInventoryChildren(&b, data, assigned, "    bastion:\n      hosts:\n        bastion-0:\n", false)

	return b.String(), nil
}

func writeNetworkVars(b *strings.Builder, data *clusterNodesJSON, sshUser string) {
	b.WriteString("all:\n  vars:\n")
	writeLine(b, "    ansible_user: ", yamlQuote(sshUser))
	writeLine(b, "    kube_api_host: ", yamlQuote(data.Metadata.KubeAPIHost))
	writeLine(b, "    fqdn: ", yamlQuote(data.Metadata.FQDN))
	writeLine(b, "    bastion_host: ", yamlQuote(data.Bastion.PublicIP))
	writeLine(b, "    bastion_user: ", yamlQuote(sshUser))
	writeLine(b, "    bastion_dns: ", yamlQuote(data.Bastion.PublicDNS))
}

func writeNetworkHosts(b *strings.Builder, data *clusterNodesJSON, env, sshUser, keyPath string,
	assigned map[string]string,
) error {
	direct := "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
	nodeArgs := direct
	if env == envIPv6 {
		// %h is bracketed and quoted so ssh -W parses an IPv6 literal and the shell never globs it.
		proxy := fmt.Sprintf(`ssh -i %s %s -W "[%%h]:%%p" %s`,
			shellQuote(keyPath), direct, shellQuote(sshUser+"@"+data.Bastion.PublicIP))
		nodeArgs = direct + " -o ProxyCommand=" + shellQuote(proxy)
	}

	b.WriteString("  hosts:\n    bastion-0:\n")
	writeLine(b, "      ansible_host: ", yamlQuote(data.Bastion.PublicIP))
	writeLine(b, "      ansible_ssh_common_args: ", yamlQuote(direct))
	for i := range data.Nodes {
		n := &data.Nodes[i]
		addr := n.PublicIP
		if env == envIPv6 {
			addr = n.IPv6
		}
		if addr == "" || n.IPv6 == "" {
			return fmt.Errorf("%s node %s is missing its address in cluster_nodes_json (public_ip=%q ipv6=%q)",
				env, n.Name, n.PublicIP, n.IPv6)
		}
		writeLine(b, "    ", n.Name, ":")
		writeLine(b, "      ansible_host: ", yamlQuote(addr))
		writeLine(b, "      ansible_host_ipv6: ", yamlQuote(n.IPv6))
		writeLine(b, "      ansible_ssh_common_args: ", yamlQuote(nodeArgs))
		writeLine(b, "      node_roles: ", yamlInlineList(n.Roles))
		writeLine(b, "      rke2_node_role: ", yamlQuote(nodeRole(n, assigned)))
	}

	return nil
}

// nodeExternalIPPatch is the config.yaml line that makes kubelet advertise the node's
// addresses. Dual-stack order follows service-cidr: kube-apiserver rejects a mismatched family.
func nodeExternalIPPatch(env string, ipv6First bool) string {
	switch env {
	case envDualstack:
		if ipv6First {
			return `node-external-ip: "{{ ansible_host_ipv6 }},{{ ansible_host }}"`
		}
		return `node-external-ip: "{{ ansible_host }},{{ ansible_host_ipv6 }}"`
	case envIPv6:
		return `node-external-ip: "{{ ansible_host_ipv6 }}"`
	default:
		return "node-external-ip: {{ ansible_host }}"
	}
}

// k3sExternalIPBlock is k3s_install's node-external-ip list, which only carries public_ip (ansible_host).
const k3sExternalIPBlock = "node-external-ip:\n  - \"{{ public_ip }}\""

// patchK3sExternalIP makes k3s dualstack/ipv6 nodes advertise the same families, in the same order, as rke2.
func patchK3sExternalIP(tmpl, env string, ipv6First bool) (string, bool) {
	if env == "" || !strings.Contains(tmpl, k3sExternalIPBlock) {
		return tmpl, false
	}

	return strings.Replace(tmpl, k3sExternalIPBlock, nodeExternalIPPatch(env, ipv6First), 1), true
}

// stageBastionKubeconfig puts the playbook's bastion kubeconfig where the
// *UsingBastion testcases read it (/tmp/<product>_kubeconf.yaml).
func stageBastionKubeconfig(config *driver.InfraConfig) error {
	if networkEnv(config.Module) == "" {
		return nil
	}
	bastion := config.Cluster.Bastion.PublicIPv4Addr
	if bastion == "" {
		return errors.New("no bastion recorded for the cluster; cannot stage its kubeconfig")
	}
	cmd := fmt.Sprintf("cp ~/.kube/config /tmp/%s_kubeconf.yaml", config.Product)
	if _, err := resources.RunCommandOnNode(cmd, bastion); err != nil {
		return fmt.Errorf("stage kubeconfig on bastion %s: %w", bastion, err)
	}
	resources.LogLevel("info", "Bastion kubeconfig staged at /tmp/%s_kubeconf.yaml", config.Product)

	return nil
}

// ipv6FirstCIDR reports whether SERVER_FLAGS' service-cidr lists an IPv6 range first.
// Only that entry is parsed: unquoted {{ }} templates elsewhere are not valid YAML.
func ipv6FirstCIDR(serverFlags string) bool {
	var flags map[string]any
	if err := yaml.Unmarshal([]byte(serviceCIDREntry(serverFlags)), &flags); err != nil {
		return false
	}
	first := ""
	switch v := flags["service-cidr"].(type) {
	case string:
		first, _, _ = strings.Cut(v, ",")
	case []any:
		if len(v) > 0 {
			first, _, _ = strings.Cut(fmt.Sprint(v[0]), ",")
		}
	}

	return resources.IsIPv6(strings.TrimSpace(first))
}

// serviceCIDREntry returns the top-level service-cidr key with its continuation lines:
// indented, "- " items, blank or comment-only.
func serviceCIDREntry(serverFlags string) string {
	var b strings.Builder
	in := false
	for _, line := range strings.Split(resources.NormalizeString(serverFlags), "\n") {
		trimmed := strings.TrimSpace(line)
		cont := trimmed == "" || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "-")
		switch {
		case strings.HasPrefix(line, "service-cidr:"):
			in = true
		case !cont:
			in = false
		}
		if in {
			b.WriteString(line + "\n")
		}
	}

	return b.String()
}
