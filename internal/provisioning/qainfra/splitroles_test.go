package qainfra

import (
	"strings"
	"testing"
)

func setNodeEnvs(t *testing.T, envs map[string]string) {
	t.Helper()
	// clear every topology env so leftovers from the host never leak into a case
	for _, k := range []string{
		"SPLIT_ROLES", "split_roles", "NO_OF_SERVER_NODES", "no_of_server_nodes",
		"NO_OF_WORKER_NODES", "no_of_worker_nodes", "ETCD_ONLY_NODES", "etcd_only_nodes",
		"ETCD_CP_NODES", "etcd_cp_nodes", "ETCD_WORKER_NODES", "etcd_worker_nodes",
		"CP_ONLY_NODES", "cp_only_nodes", "CP_WORKER_NODES", "cp_worker_nodes",
	} {
		t.Setenv(k, "")
	}
	for k, v := range envs {
		t.Setenv(k, v)
	}
}

func TestBuildNodesTFVarOmitsZeroCounts(t *testing.T) {
	// split-role job shape that broke destroy: zero all-roles servers, one worker,
	// counts coming from the role envs — no count=0 entry may reach tofu.
	setNodeEnvs(t, map[string]string{
		"SPLIT_ROLES":        "true",
		"NO_OF_SERVER_NODES": "0",
		"NO_OF_WORKER_NODES": "1",
		"ETCD_ONLY_NODES":    "3",
		"ETCD_CP_NODES":      "1",
		"ETCD_WORKER_NODES":  "0",
		"CP_ONLY_NODES":      "1",
		"CP_WORKER_NODES":    "0",
	})

	out, err := buildNodesTFVar()
	if err != nil {
		t.Fatalf("buildNodesTFVar: %v", err)
	}
	// exact-shape assertion: counts bound to their role groups, zero-count entries gone
	want := `[{"count":3,"role":["etcd"]},{"count":1,"role":["etcd","cp"]},` +
		`{"count":1,"role":["cp"]},{"count":1,"role":["worker"]}]`
	if out != want {
		t.Fatalf("unexpected nodes JSON:\nwant: %s\ngot:  %s", want, out)
	}
}

func TestBuildNodesTFVarSimpleZeroWorkers(t *testing.T) {
	setNodeEnvs(t, map[string]string{
		"NO_OF_SERVER_NODES": "1",
		"NO_OF_WORKER_NODES": "0",
	})

	out, err := buildNodesTFVar()
	if err != nil {
		t.Fatalf("buildNodesTFVar: %v", err)
	}
	want := `[{"count":1,"role":["etcd","cp","worker"]}]`
	if out != want {
		t.Fatalf("unexpected nodes JSON:\nwant: %s\ngot:  %s", want, out)
	}
}

func TestAppendNodesVarApplyDestroySymmetry(t *testing.T) {
	// destroy must carry the exact same serialized -var=nodes= as apply,
	// split roles included — asymmetry is the bug that leaked clusters.
	setNodeEnvs(t, map[string]string{
		"SPLIT_ROLES":        "true",
		"NO_OF_WORKER_NODES": "1",
		"ETCD_ONLY_NODES":    "3",
		"ETCD_CP_NODES":      "1",
		"CP_ONLY_NODES":      "1",
	})

	lastVar := func(args []string) string {
		for _, a := range args {
			if strings.HasPrefix(a, "-var=nodes=") {
				return a
			}
		}

		return ""
	}

	applyArgs, err := appendNodesVar([]string{"apply", "-auto-approve", "-var-file=x"})
	if err != nil {
		t.Fatalf("apply args: %v", err)
	}
	destroyArgs, err := appendNodesVar([]string{"destroy", "-auto-approve", "-var-file=vars.tfvars"})
	if err != nil {
		t.Fatalf("destroy args: %v", err)
	}

	a, d := lastVar(applyArgs), lastVar(destroyArgs)
	if a == "" || d == "" {
		t.Fatalf("nodes var missing: apply=%q destroy=%q", a, d)
	}
	if a != d {
		t.Fatalf("apply/destroy nodes var diverged:\napply:   %s\ndestroy: %s", a, d)
	}
}

func TestBuildNodesTFVarSplitWithoutCountsFails(t *testing.T) {
	setNodeEnvs(t, map[string]string{"SPLIT_ROLES": "true"})
	if _, err := buildNodesTFVar(); err == nil {
		t.Fatal("expected error when split_roles=true with no counts")
	}
}

func TestTfvarsValueStripsInlineComments(t *testing.T) {
	cases := map[string]string{
		`"ami-04d0934033d5d6964" # RHEL 9.8`: "ami-04d0934033d5d6964",
		`"us-east-2"`:                        "us-east-2",
		`50`:                                 "50",
		`50 # gp3 default`:                   "50",
		`"a#b" # real hash inside quotes`:    "a#b",
	}
	for in, want := range cases {
		if got := tfvarsValue(in); got != want {
			t.Errorf("tfvarsValue(%q) = %q, want %q", in, got, want)
		}
	}
}
