package qainfra

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
		return nil, errors.New("cluster_nodes_json output is empty — did 'tofu apply' complete?")
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
// master -> first node with role 'etcd' (k3s: first node with role 'cp');
// servers -> nodes with role 'cp'; workers -> nodes with role 'worker'.
//
// Per-host vars: ansible_host, node_roles, rke2_node_role.
// All vars: ansible_user, ansible_ssh_common_args, kube_api_host, fqdn.
func buildStaticInventory(data *clusterNodesJSON, product string) string {
	assigned := assignNodeGroups(data, product)

	var b strings.Builder
	writeInventoryVars(&b, data)
	writeInventoryHosts(&b, data, assigned)
	writeInventoryChildren(&b, data, assigned)

	return b.String()
}

// assignNodeGroups maps each node name to its inventory group (master/servers/
// workers), applying the mutually-exclusive first-match-wins rules.
func assignNodeGroups(data *clusterNodesJSON, product string) map[string]string {
	masterRoles := []string{"etcd"}
	if strings.EqualFold(product, "k3s") {
		masterRoles = []string{"cp"}
	}

	assigned := make(map[string]string) // node name -> group
	for _, n := range data.Nodes {
		if hasAnyRole(n.Roles, masterRoles) {
			assigned[n.Name] = "master"

			break
		}
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

	return assigned
}

// nodeRole returns the rke2_node_role value for a node.
func nodeRole(n clusterNode, assigned map[string]string) string {
	switch {
	case assigned[n.Name] == "master":
		return "master"
	case hasAnyRole(n.Roles, []string{"cp", "etcd"}):
		return "server"
	default:
		return "agent"
	}
}

// writeLine writes the given parts followed by a newline, avoiding an
// intermediate concatenated string.
func writeLine(b *strings.Builder, parts ...string) {
	for _, p := range parts {
		b.WriteString(p)
	}
	b.WriteString("\n")
}

// writeInventoryVars writes the all.vars block.
func writeInventoryVars(b *strings.Builder, data *clusterNodesJSON) {
	b.WriteString("all:\n")
	b.WriteString("  vars:\n")
	b.WriteString(`    ansible_ssh_common_args: "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"` + "\n")
	if data.Metadata.SSHUser != "" {
		writeLine(b, "    ansible_user: ", yamlQuote(data.Metadata.SSHUser))
	}
	if data.Metadata.KubeAPIHost != "" {
		writeLine(b, "    kube_api_host: ", yamlQuote(data.Metadata.KubeAPIHost))
	}
	if data.Metadata.FQDN != "" {
		writeLine(b, "    fqdn: ", yamlQuote(data.Metadata.FQDN))
	}
	if data.Metadata.SSHPrivateKey != "" {
		writeLine(b, "    ansible_ssh_private_key_file: ", yamlQuote(data.Metadata.SSHPrivateKey))
	}
}

// writeInventoryHosts writes the all.hosts block.
func writeInventoryHosts(b *strings.Builder, data *clusterNodesJSON, assigned map[string]string) {
	b.WriteString("  hosts:\n")
	for _, n := range data.Nodes {
		writeLine(b, "    ", n.Name, ":")
		writeLine(b, "      ansible_host: ", yamlQuote(n.PublicIP))
		writeLine(b, "      node_roles: ", yamlInlineList(n.Roles))
		writeLine(b, "      rke2_node_role: ", yamlQuote(nodeRole(n, assigned)))
	}
}

// writeInventoryChildren writes the all.children groups (master/servers/workers).
func writeInventoryChildren(b *strings.Builder, data *clusterNodesJSON, assigned map[string]string) {
	groups := []string{"master", "servers", "workers"}
	groupMembers := make(map[string][]clusterNode)
	for _, n := range data.Nodes {
		if g, ok := assigned[n.Name]; ok {
			groupMembers[g] = append(groupMembers[g], n)
		}
	}

	anyChildren := false
	for _, g := range groups {
		if len(groupMembers[g]) > 0 {
			anyChildren = true

			break
		}
	}
	if !anyChildren {
		return
	}

	b.WriteString("  children:\n")
	for _, g := range groups {
		members := groupMembers[g]
		if len(members) == 0 {
			continue
		}
		sort.SliceStable(members, func(i, j int) bool { return members[i].Name < members[j].Name })
		writeLine(b, "    ", g, ":")
		b.WriteString("      hosts:\n")
		for _, n := range members {
			writeLine(b, "        ", n.Name, ":")
			writeLine(b, "          ansible_host: ", yamlQuote(n.PublicIP))
		}
	}
}

func hasAnyRole(have, want []string) bool {
	for _, h := range have {
		if slices.Contains(want, h) {
			return true
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
