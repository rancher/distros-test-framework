package support

import (
	"context"
	"errors"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rancher/distros-test-framework/config"
)

func kataConfigForTest(t *testing.T) *config.Env {
	t.Helper()
	for _, key := range []string{
		"KUBE_CONFIG", "SPLIT_ROLES", "split_roles", "no_of_server_nodes", "no_of_worker_nodes",
		"INSTALL_MODE", "install_mode", "QA_INFRA_WORKER_NESTED",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("NO_OF_SERVER_NODES", "1")
	t.Setenv("NO_OF_WORKER_NODES", "2")

	return &config.Env{
		Product: "rke2", ProvisionerModule: "qainfra", QAInfraProvider: "aws", Arch: "amd64",
		NodeOS: "rhel10", InstallVersion: "v1.37.2+rke2r1", Channel: "stable",
		ServerFlags: "selinux: true\nprofile: cis\nprime: true", WorkerFlags: "selinux: true\nprofile: cis",
	}
}

func TestLoadKataConfigSecurityPolicy(t *testing.T) {
	const staging = "stgregistry.suse.com"
	for _, tc := range []struct {
		name, nodeOS, version, channel, mode, registry, build, lsm string
	}{
		{"GA SELinux", "rhel10", "v1.37.2+rke2r1", "stable", "", "", "ga", "selinux"},
		{"SLES SELinux", "sles16", "v1.37.2+rke2r1", "latest", "", "", "ga", "selinux"},
		{"Ubuntu AppArmor", "ubuntu", "v1.37.2+rke2r1", "stable", "", "", "ga", "apparmor"},
		{"SLES AppArmor", "sles15", "v1.37.2+rke2r1", "stable", "", "", "ga", "apparmor"},
		{"RC", "rhel10", "v1.37.2-rc1+rke2r1", "testing", "", staging, "staging", "selinux"},
		{"testing release-shaped tag", "rhel10", "v1.37.2+rke2r1", "testing", "", staging, "staging", "selinux"},
		{"commit", "rhel10", "3f2a1bc9d8e7", "latest", "COMMIT", staging, "staging", "selinux"},
		{"explicit staging override", "rhel10", "v1.37.2+rke2r1", "stable", "", staging, "staging", "selinux"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := kataConfigForTest(t)
			cfg.NodeOS, cfg.InstallVersion, cfg.Channel = tc.nodeOS, tc.version, tc.channel
			cfg.ServerFlags += "\nsystem-default-registry: " + tc.registry
			t.Setenv("INSTALL_MODE", tc.mode)

			got, err := LoadKataConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			registry := tc.registry
			if registry == "" {
				registry = "registry.rancher.com"
			}
			want := kataSecurityPolicy{tc.build, tc.version, tc.channel, registry, tc.lsm}
			if got.Security != want {
				t.Errorf("security policy = %+v; want %+v", got.Security, want)
			}
			if os.Getenv("QA_INFRA_WORKER_NESTED") != "true" {
				t.Error("validated Kata run did not enable nested virtualization")
			}
		})
	}
}

func TestLoadKataConfigRejectsInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*config.Env)
		env    map[string]string
	}{
		{name: "wrong product", change: func(c *config.Env) { c.Product = "k3s" }},
		{name: "wrong provider", change: func(c *config.Env) { c.QAInfraProvider = "vsphere" }},
		{name: "wrong architecture", change: func(c *config.Env) { c.Arch = "arm64" }},
		{name: "existing cluster", env: map[string]string{"KUBE_CONFIG": "/tmp/existing-kubeconfig"}},
		{name: "missing worker", env: map[string]string{"NO_OF_WORKER_NODES": "1"}},
		{name: "conflicting node counts", env: map[string]string{"no_of_server_nodes": "3"}},
		{name: "split roles", env: map[string]string{"split_roles": "true"}},
		{name: "missing version", change: func(c *config.Env) { c.InstallVersion = "" }},
		{name: "missing channel", change: func(c *config.Env) { c.Channel = "" }},
		{name: "unapproved OS", change: func(c *config.Env) { c.NodeOS = "rhel9" }},
		{name: "missing server Prime", change: func(c *config.Env) { c.ServerFlags = c.WorkerFlags }},
		{name: "Prime on worker", change: func(c *config.Env) { c.WorkerFlags += "\nprime: false" }},
		{name: "missing worker SELinux", change: func(c *config.Env) { c.WorkerFlags = "profile: cis" }},
		{name: "missing worker CIS", change: func(c *config.Env) { c.WorkerFlags = "selinux: true" }},
		{name: "staging without registry", change: func(c *config.Env) { c.Channel = "testing" }},
		{name: "RC without registry", change: func(c *config.Env) { c.InstallVersion = "v1.37.2-rc1+rke2r1" }},
		{name: "commit without registry", env: map[string]string{"INSTALL_MODE": "INSTALL_RKE2_COMMIT"}},
		{name: "wrong registry", change: func(c *config.Env) {
			c.WorkerFlags += "\nsystem-default-registry: other.invalid"
		}},
		{name: "unsupported snapshotter", change: func(c *config.Env) { c.WorkerFlags += "\nsnapshotter: native" }},
		{name: "invalid YAML", change: func(c *config.Env) { c.ServerFlags = "selinux: [" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := kataConfigForTest(t)
			if tc.change != nil {
				tc.change(cfg)
			}
			for key, value := range tc.env {
				t.Setenv(key, value)
			}

			if _, err := LoadKataConfig(cfg); err == nil {
				t.Fatal("invalid configuration was accepted")
			}
			if os.Getenv("QA_INFRA_WORKER_NESTED") != "" {
				t.Error("rejected configuration changed nested virtualization settings")
			}
		})
	}
}

func TestPollKataRetriesUntilReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		attempts := 0
		err := pollKata(t.Context(), time.Minute, func(context.Context) error {
			attempts++
			if attempts == 1 {
				return errors.New("pod is pending")
			}

			return nil
		})
		if err != nil || attempts != 2 {
			t.Fatalf("poll returned %v after %d attempts; want success after retry", err, attempts)
		}
	})
}

func TestPollKataTimeoutPreservesCause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		notReady := errors.New("sandbox has no VM")
		err := pollKata(t.Context(), time.Second, func(ctx context.Context) error {
			<-ctx.Done()

			return notReady
		})
		if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, notReady) {
			t.Fatalf("timeout lost deadline or last failure: %v", err)
		}
	})
}

func TestPollKataCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := pollKata(ctx, time.Minute, func(context.Context) error {
		t.Fatal("poll started work after cancellation")

		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was not propagated: %v", err)
	}
}
