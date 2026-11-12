package qainfra

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// Dual-stack and IPv6-only runs use qa-infra's tofu/aws/modules/<env> and
// ansible/rke2/<env> instead of cluster_nodes and ansible/<product>/default.
const (
	dualstackModule = "dualstack"
	ipv6OnlyModule  = "ipv6only"
	envDualstack    = "dualstack"
	envIPv6         = "ipv6"
)

// The dualstack/ipv6 modules do not declare these cluster_nodes inputs.
var clusterNodesOnlyArgsRE = regexp.MustCompile(`(?m)^[ \t]*(airgap_setup|proxy_setup)[ \t]*=.*\n`)

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

// validateNetworkEnv rejects combinations qa-infra has no module or playbook for.
func validateNetworkEnv(product, provider, env string) error {
	if env == "" {
		return nil
	}
	if !strings.EqualFold(product, "rke2") {
		return fmt.Errorf("qainfra %s runs support only rke2 (qa-infra has no ansible/%s/%s playbook)",
			env, strings.ToLower(product), env)
	}
	if provider != "" && !strings.EqualFold(provider, "aws") {
		return fmt.Errorf("qainfra %s runs support only QA_INFRA_PROVIDER=aws, got %q", env, provider)
	}

	return nil
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
	direct := "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
	nodeArgs := direct
	if env == envIPv6 {
		// %h is bracketed so ssh -W can parse an IPv6 literal.
		nodeArgs = fmt.Sprintf("%s -o ProxyCommand='ssh -i %s %s -W [%%h]:%%p %s@%s'",
			direct, keyPath, direct, sshUser, data.Bastion.PublicIP)
	}
	assigned := assignNodeGroups(data, "")

	var b strings.Builder
	b.WriteString("all:\n  vars:\n")
	writeLine(&b, "    ansible_user: ", yamlQuote(sshUser))
	writeLine(&b, "    kube_api_host: ", yamlQuote(data.Metadata.KubeAPIHost))
	writeLine(&b, "    fqdn: ", yamlQuote(data.Metadata.FQDN))
	writeLine(&b, "    bastion_host: ", yamlQuote(data.Bastion.PublicIP))
	writeLine(&b, "    bastion_user: ", yamlQuote(sshUser))
	writeLine(&b, "    bastion_dns: ", yamlQuote(data.Bastion.PublicDNS))
	b.WriteString("  hosts:\n    bastion-0:\n")
	writeLine(&b, "      ansible_host: ", yamlQuote(data.Bastion.PublicIP))
	writeLine(&b, "      ansible_ssh_common_args: ", yamlQuote(direct))
	for i := range data.Nodes {
		n := &data.Nodes[i]
		addr := n.PublicIP
		if env == envIPv6 {
			addr = n.IPv6
		}
		if addr == "" || n.IPv6 == "" {
			return "", fmt.Errorf("%s node %s is missing its address in cluster_nodes_json (public_ip=%q ipv6=%q)",
				env, n.Name, n.PublicIP, n.IPv6)
		}
		writeLine(&b, "    ", n.Name, ":")
		writeLine(&b, "      ansible_host: ", yamlQuote(addr))
		writeLine(&b, "      ansible_host_ipv6: ", yamlQuote(n.IPv6))
		writeLine(&b, "      ansible_ssh_common_args: ", yamlQuote(nodeArgs))
		writeLine(&b, "      node_roles: ", yamlInlineList(n.Roles))
		writeLine(&b, "      rke2_node_role: ", yamlQuote(nodeRole(n, assigned)))
	}
	b.WriteString("  children:\n    bastion:\n      hosts:\n        bastion-0:\n")
	for _, g := range []string{"master", "servers", "workers"} {
		var members []string
		for _, n := range data.Nodes {
			if assigned[n.Name] == g {
				members = append(members, n.Name)
			}
		}
		if len(members) == 0 {
			continue
		}
		writeLine(&b, "    ", g, ":\n      hosts:")
		for _, m := range members {
			writeLine(&b, "        ", m, ":")
		}
	}

	return b.String(), nil
}

// nodeExternalIPPatch is the config.yaml line that makes kubelet advertise the
// node's reachable address(es); dual-stack tests curl both families.
func nodeExternalIPPatch(env string) string {
	switch env {
	case envDualstack:
		return `node-external-ip: "{{ ansible_host }},{{ ansible_host_ipv6 }}"`
	case envIPv6:
		return `node-external-ip: "{{ ansible_host_ipv6 }}"`
	default:
		return "node-external-ip: {{ ansible_host }}"
	}
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
