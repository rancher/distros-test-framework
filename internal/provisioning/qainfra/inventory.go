package qainfra

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// clusterNodesJSON mirrors the cluster_nodes_json output that
// rancher/qa-infra-automation's Tofu cluster_nodes module emits. The contract
// is documented at docs/reference/inventory-format.md and validated by their
// scripts/generate_inventory.py.
type clusterNodesJSON struct {
	Type     string `json:"type"`
	Metadata struct {
		KubeAPIHost   string `json:"kube_api_host"`
		FQDN          string `json:"fqdn"`
		SSHUser       string `json:"ssh_user"`
		SSHPrivateKey string `json:"ssh_private_key,omitempty"`
	} `json:"metadata"`
	Nodes []clusterNode `json:"nodes"`
}

type clusterNode struct {
	Name      string   `json:"name"`
	Roles     []string `json:"roles"`
	PublicIP  string   `json:"public_ip"`
	PrivateIP string   `json:"private_ip"`
}

// fetchClusterNodesJSON reads the Tofu cluster_nodes_json output from the
// already-applied module and decodes it. Tofu stays the source of truth —
// we just stop relying on the Ansible cloud.terraform plugin to do the
// reading at playbook runtime.
func fetchClusterNodesJSON(tofuDir string) (*clusterNodesJSON, error) {
	cmd := exec.Command("tofu", "output", "-raw", "cluster_nodes_json")
	cmd.Dir = tofuDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tofu output -raw cluster_nodes_json: %w (stderr: %s)",
			err, strings.TrimSpace(stderr.String()))
	}

	raw := bytes.TrimSpace(stdout.Bytes())
	if len(raw) == 0 {
		return nil, fmt.Errorf("cluster_nodes_json output is empty — did 'tofu apply' complete?")
	}

	var data clusterNodesJSON
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("unmarshal cluster_nodes_json: %w", err)
	}
	return &data, nil
}

// buildStaticInventory turns a cluster_nodes_json payload into a static
// Ansible inventory YAML, matching the shape produced by upstream's
// scripts/generate_inventory.py for the {rke2,k3s}/default schemas.
//
// Group rules (mutually exclusive, first match wins in this order):
//
//	master  -> first node with role 'etcd' (k3s: first node with role 'cp')
//	servers -> nodes with role 'cp'
//	workers -> nodes with role 'worker'
//
// Per-host vars:  ansible_host, node_roles, rke2_node_role
// All vars:       ansible_user, ansible_ssh_common_args, kube_api_host, fqdn
func buildStaticInventory(data *clusterNodesJSON, product string) string {
	masterRoles := []string{"etcd"}
	if strings.EqualFold(product, "k3s") {
		masterRoles = []string{"cp"}
	}

	assigned := make(map[string]string) // node name -> group
	pickFirst := func(roles []string) string {
		for _, n := range data.Nodes {
			if hasAnyRole(n.Roles, roles) {
				return n.Name
			}
		}
		return ""
	}
	if m := pickFirst(masterRoles); m != "" {
		assigned[m] = "master"
	}
	for _, n := range data.Nodes {
		if _, ok := assigned[n.Name]; ok {
			continue
		}
		switch {
		case hasAnyRole(n.Roles, []string{"cp"}):
			assigned[n.Name] = "servers"
		case hasAnyRole(n.Roles, []string{"worker"}):
			assigned[n.Name] = "workers"
		}
	}

	roleFor := func(n clusterNode) string {
		switch {
		case assigned[n.Name] == "master":
			return "master"
		case hasAnyRole(n.Roles, []string{"cp", "etcd"}):
			return "server"
		default:
			return "agent"
		}
	}

	var b strings.Builder
	fmt.Fprintln(&b, "all:")
	fmt.Fprintln(&b, "  vars:")
	fmt.Fprintln(&b, `    ansible_ssh_common_args: "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"`)
	if data.Metadata.SSHUser != "" {
		fmt.Fprintf(&b, "    ansible_user: %s\n", yamlQuote(data.Metadata.SSHUser))
	}
	if data.Metadata.KubeAPIHost != "" {
		fmt.Fprintf(&b, "    kube_api_host: %s\n", yamlQuote(data.Metadata.KubeAPIHost))
	}
	if data.Metadata.FQDN != "" {
		fmt.Fprintf(&b, "    fqdn: %s\n", yamlQuote(data.Metadata.FQDN))
	}
	if data.Metadata.SSHPrivateKey != "" {
		fmt.Fprintf(&b, "    ansible_ssh_private_key_file: %s\n", yamlQuote(data.Metadata.SSHPrivateKey))
	}

	fmt.Fprintln(&b, "  hosts:")
	for _, n := range data.Nodes {
		fmt.Fprintf(&b, "    %s:\n", n.Name)
		fmt.Fprintf(&b, "      ansible_host: %s\n", yamlQuote(n.PublicIP))
		fmt.Fprintf(&b, "      node_roles: %s\n", yamlInlineList(n.Roles))
		fmt.Fprintf(&b, "      rke2_node_role: %s\n", yamlQuote(roleFor(n)))
	}

	groups := []string{"master", "servers", "workers"}
	groupMembers := make(map[string][]clusterNode)
	for _, n := range data.Nodes {
		g, ok := assigned[n.Name]
		if !ok {
			continue
		}
		groupMembers[g] = append(groupMembers[g], n)
	}
	var anyChildren bool
	for _, g := range groups {
		if len(groupMembers[g]) > 0 {
			anyChildren = true
			break
		}
	}
	if anyChildren {
		fmt.Fprintln(&b, "  children:")
		for _, g := range groups {
			members := groupMembers[g]
			if len(members) == 0 {
				continue
			}
			sort.SliceStable(members, func(i, j int) bool { return members[i].Name < members[j].Name })
			fmt.Fprintf(&b, "    %s:\n", g)
			fmt.Fprintln(&b, "      hosts:")
			for _, n := range members {
				fmt.Fprintf(&b, "        %s:\n", n.Name)
				fmt.Fprintf(&b, "          ansible_host: %s\n", yamlQuote(n.PublicIP))
			}
		}
	}

	return b.String()
}

func hasAnyRole(have, want []string) bool {
	for _, h := range have {
		for _, w := range want {
			if h == w {
				return true
			}
		}
	}
	return false
}

// yamlQuote wraps a scalar in double quotes and escapes embedded quotes /
// backslashes. Cheap-and-cheerful — sufficient for the IPs, FQDNs, SSH user
// strings and paths we deal with here.
func yamlQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func yamlInlineList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	quoted := make([]string, len(items))
	for i, it := range items {
		quoted[i] = yamlQuote(it)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// writeStaticInventory ties fetch + build + write into a single call site.
// Logs a sanitized summary so failures during the run point at the right node.
func writeStaticInventory(config *driver.InfraConfig) error {
	data, err := fetchClusterNodesJSON(config.InfraProvisioner.TFNodeSource)
	if err != nil {
		return fmt.Errorf("fetch cluster_nodes_json: %w", err)
	}

	yaml := buildStaticInventory(data, config.Product)

	resources.LogLevel("info",
		"Static inventory: %d node(s); kube_api_host=%s fqdn=%s",
		len(data.Nodes), data.Metadata.KubeAPIHost, data.Metadata.FQDN)

	if err := os.MkdirAll(filepath.Dir(config.InfraProvisioner.Inventory.Path), 0o755); err != nil {
		return fmt.Errorf("create inventory dir: %w", err)
	}
	if err := os.WriteFile(config.InfraProvisioner.Inventory.Path, []byte(yaml), 0o644); err != nil {
		return fmt.Errorf("write inventory: %w", err)
	}
	return nil
}
