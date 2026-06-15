package qainfra

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

// envLower reads an env var, lowercase first (legacy contract), uppercase as
// alias. The legacy framework and upgradesuc.go both read lowercase
// (`split_roles`, `etcd_only_nodes`), so we keep that as canonical and treat
// uppercase as a convenience for users with shell-style env files.
func envLower(name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}

	return os.Getenv(strings.ToUpper(name))
}

// envInt parses an env var as a non-negative int. Returns def when unset;
// returns an error when set but unparseable or negative — fail loud rather
// than silently default a typo or accept a value the `> 0` callers would
// silently ignore.
func envInt(name string, def int) (int, error) {
	v := envLower(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("env %s=%q is not a valid integer: %w", name, v, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("env %s=%q must be >= 0", name, v)
	}

	return n, nil
}

type nodeEntry struct {
	Count int      `json:"count"`
	Role  []string `json:"role"`
}

// buildNodesTFVar translates legacy split-roles env vars into the qainfra
// `nodes` list shape, returned as a JSON string for `tofu apply -var=nodes=…`.
// `role_order` is intentionally not read. On qainfra, install order is
// handled by the upstream Ansible play (etcd cluster-init → etcd peers → cp
// → workers) and ServerIPs[] is sorted master-first then alphabetic in
// nodeextractor.go. Honoring role_order would require threading it into the
// sort.SliceStable predicate there.
func buildNodesTFVar() (string, error) {
	splitRoles := envLower("split_roles") == "true"

	var (
		entries []nodeEntry
		err     error
	)
	if splitRoles {
		entries, err = buildSplitRoleEntries()
	} else {
		entries, err = buildSimpleEntries()
	}
	if err != nil {
		return "", err
	}

	// Fail closed when split_roles=true but the user produced no entries —
	// otherwise tofu would silently fall back to vars.tfvars's all-roles
	// default and the split roles test would pass against the wrong topology.
	if splitRoles && len(entries) == 0 {
		return "", errors.New("split_roles=true but no role counts set — " +
			"provide at least one of etcd_only_nodes / etcd_cp_nodes / " +
			"etcd_worker_nodes / cp_only_nodes / cp_worker_nodes (>0)")
	}

	if len(entries) == 0 {
		return "", nil
	}

	if !hasEtcdNode(entries) {
		return "", errors.New("topology has no etcd node — cluster cannot bootstrap")
	}

	out, err := json.Marshal(entries)
	if err != nil {
		return "", fmt.Errorf("marshal nodes: %w", err)
	}

	return string(out), nil
}

// buildSplitRoleEntries builds the nodes list for split_roles=true. Returns
// (nil, nil) when every count is unset or zero .
func buildSplitRoleEntries() ([]nodeEntry, error) {
	roleMap := []struct {
		envKey string
		roles  []string
	}{
		{"etcd_only_nodes", []string{"etcd"}},
		{"etcd_cp_nodes", []string{"etcd", "cp"}},
		{"etcd_worker_nodes", []string{"etcd", "worker"}},
		{"cp_only_nodes", []string{"cp"}},
		{"cp_worker_nodes", []string{"cp", "worker"}},
	}

	var entries []nodeEntry
	for _, r := range roleMap {
		n, err := envInt(r.envKey, 0)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			entries = append(entries, nodeEntry{Count: n, Role: r.roles})
		}
	}

	// Allow all-roles servers alongside split-role nodes (legacy supported
	// this via no_of_server_nodes summing into NumServers).
	servers, err := envInt("no_of_server_nodes", 0)
	if err != nil {
		return nil, err
	}
	if servers > 0 {
		entries = append(entries, nodeEntry{Count: servers, Role: []string{"etcd", "cp", "worker"}})
	}

	workers, err := envInt("no_of_worker_nodes", 0)
	if err != nil {
		return nil, err
	}
	if workers > 0 {
		entries = append(entries, nodeEntry{Count: workers, Role: []string{"worker"}})
	}

	return entries, nil
}

// buildSimpleEntries builds the nodes list when split_roles is false or unset.
// Returns (nil, nil) when neither count is set — that signals "use the
// vars.tfvars default".
func buildSimpleEntries() ([]nodeEntry, error) {
	servers, err := envInt("no_of_server_nodes", 0)
	if err != nil {
		return nil, err
	}
	workers, err := envInt("no_of_worker_nodes", 0)
	if err != nil {
		return nil, err
	}

	if servers == 0 && workers == 0 {
		return nil, nil
	}

	var entries []nodeEntry
	if servers > 0 {
		entries = append(entries, nodeEntry{Count: servers, Role: []string{"etcd", "cp", "worker"}})
	}
	if workers > 0 {
		entries = append(entries, nodeEntry{Count: workers, Role: []string{"worker"}})
	}

	return entries, nil
}

func hasEtcdNode(entries []nodeEntry) bool {
	for _, e := range entries {
		for _, r := range e.Role {
			if r == "etcd" {
				return true
			}
		}
	}

	return false
}

// applySplitRolesIfEnabled fills cluster.Config.SplitRoles by classifying each
// extracted node's role-combo. Keeps the legacy report.go summary path working
// for qainfra runs that use split-roles.
func applySplitRolesIfEnabled(config *driver.InfraConfig, nodes []infraNode) {
	sp := &config.Cluster.Config.SplitRoles
	sp.Enabled = envLower("split_roles") == "true"
	if !sp.Enabled {
		return
	}

	for _, n := range nodes {
		hasEtcd := strings.Contains(n.role, "etcd")
		hasCP := strings.Contains(n.role, "cp")
		hasWorker := strings.Contains(n.role, "worker")

		switch {
		case hasEtcd && hasCP && hasWorker:
			// all-roles — counts toward NumServers but doesn't fit a
			// granular SplitRoles bucket. Skip.
		case hasEtcd && !hasCP && !hasWorker:
			sp.EtcdOnly++
		case hasEtcd && hasCP && !hasWorker:
			sp.EtcdCP++
		case hasEtcd && hasWorker && !hasCP:
			sp.EtcdWorker++
		case hasCP && !hasEtcd && !hasWorker:
			sp.ControlPlaneOnly++
		case hasCP && hasWorker && !hasEtcd:
			sp.ControlPlaneWorker++
		}
	}

	sp.NumServers = sp.EtcdOnly + sp.EtcdCP + sp.EtcdWorker + sp.ControlPlaneOnly + sp.ControlPlaneWorker
}
