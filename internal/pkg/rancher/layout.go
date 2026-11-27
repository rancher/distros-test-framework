package rancher

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Default node layouts: roles joined by "+", groups as roles:count.
const (
	DefaultCustomLayout = "etcd:3,controlplane:2,worker:1"
	DefaultImportLayout = "etcd+controlplane:3,worker:1"
)

// maxLayoutNodes caps one layout so a typo can't launch a fleet of EC2 instances.
const maxLayoutNodes = 10

var roleOrder = []string{"etcd", "controlplane", "worker"}

// ParseLayout expands a layout such as "etcd:3,cp:2,worker:1" into one role list per node.
func ParseLayout(spec string) ([][]string, error) {
	var nodes [][]string
	for _, group := range strings.Split(spec, ",") {
		rolesPart, countPart, ok := strings.Cut(strings.TrimSpace(group), ":")
		if !ok {
			return nil, fmt.Errorf("layout group %q: want roles:count, e.g. etcd+controlplane:3", group)
		}
		roles, err := parseRoles(rolesPart)
		if err != nil {
			return nil, fmt.Errorf("layout group %q: %w", group, err)
		}
		count, err := strconv.Atoi(strings.TrimSpace(countPart))
		if err != nil || count < 1 {
			return nil, fmt.Errorf("layout group %q: count must be a positive integer", group)
		}
		for range count {
			nodes = append(nodes, roles)
		}
	}
	if len(nodes) > maxLayoutNodes {
		return nil, fmt.Errorf("layout %q has %d nodes, more than the %d allowed", spec, len(nodes), maxLayoutNodes)
	}

	return nodes, nil
}

// ValidateCustomLayout checks Rancher can bring a custom cluster with this layout to Active.
func ValidateCustomLayout(nodes [][]string) error {
	for _, role := range roleOrder {
		if CountRole(nodes, role) == 0 {
			return fmt.Errorf("custom cluster layout needs at least one %s node", role)
		}
	}

	return nil
}

// ValidateImportLayout accepts only what a standalone install produces: etcd+controlplane servers and worker agents.
func ValidateImportLayout(nodes [][]string) error {
	for _, roles := range nodes {
		if !IsServer(roles) && strings.Join(roles, "+") != "worker" {
			return fmt.Errorf("import layout role set %q: use etcd+controlplane (server) or worker (agent)",
				strings.Join(roles, "+"))
		}
	}
	if len(nodes) == 0 || !IsServer(nodes[0]) {
		return errors.New("import layout must start with an etcd+controlplane group")
	}

	return nil
}

// IsServer reports whether roles is exactly etcd+controlplane, a k3s/rke2 server.
func IsServer(roles []string) bool {
	return strings.Join(roles, "+") == "etcd+controlplane"
}

// CountRole counts nodes carrying role.
func CountRole(nodes [][]string, role string) int {
	n := 0
	for _, roles := range nodes {
		for _, r := range roles {
			if r == role {
				n++
			}
		}
	}

	return n
}

// parseRoles returns the roles in canonical order so "worker+etcd" and "etcd+worker" match.
func parseRoles(s string) ([]string, error) {
	seen := map[string]bool{}
	for _, r := range strings.Split(s, "+") {
		r = strings.ToLower(strings.TrimSpace(r))
		if r == "cp" {
			r = "controlplane"
		}
		if r != "etcd" && r != "controlplane" && r != "worker" {
			return nil, fmt.Errorf("unknown role %q: use etcd, controlplane (cp) or worker", r)
		}
		seen[r] = true
	}

	var roles []string
	for _, r := range roleOrder {
		if seen[r] {
			roles = append(roles, r)
		}
	}

	return roles, nil
}
