package testcase

import (
	"context"

	"github.com/rancher/distros-test-framework/internal/pkg/testcase/support"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"

	. "github.com/onsi/gomega"
)

// TestKataInstall installs CPU-only Kata on the selected freshly provisioned worker.
func TestKataInstall(cluster *driver.Cluster, cfg *support.KataConfig) {
	Expect(support.StartKata(context.Background(), cluster, cfg)).To(Succeed())
}

// TestKataRuntime verifies explicit and alias RuntimeClasses, the shim and its KVM guest.
func TestKataRuntime(cfg *support.KataConfig) {
	s, err := support.RequireKata(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(s.RuntimeIdentity(context.Background())).To(Succeed())
}

// TestKataCoexistence checks same-node and cross-node DNS/HTTP in both runtime directions.
func TestKataCoexistence(cfg *support.KataConfig) {
	s, err := support.RequireKata(cfg, "kata-local", "runc-local", "runc-remote")
	Expect(err).NotTo(HaveOccurred())
	Expect(s.Coexistence(context.Background())).To(Succeed())
}

// TestKataRejectIneligible verifies selector-specific rejection without a sandbox.
func TestKataRejectIneligible(cfg *support.KataConfig) {
	s, err := support.RequireKata(cfg, "runc-remote")
	Expect(err).NotTo(HaveOccurred())
	Expect(s.RejectIneligible(context.Background())).To(Succeed())
}

// TestKataRestart verifies previous and newly created workloads after restarting the agent.
func TestKataRestart(cfg *support.KataConfig) {
	s, err := support.RequireKata(cfg, "kata-local", "runc-local", "runc-remote")
	Expect(err).NotTo(HaveOccurred())
	Expect(s.Restart(context.Background())).To(Succeed())
}

// TestKataIsolation checks basic guest boundaries, not a security certification.
func TestKataIsolation(cfg *support.KataConfig) {
	s, err := support.RequireKata(cfg, "kata-local")
	Expect(err).NotTo(HaveOccurred())
	Expect(s.Isolation(context.Background())).To(Succeed())
}

// TestKataPerformance records cold/warm startup and idle overhead without performance gates.
func TestKataPerformance(cfg *support.KataConfig) {
	s, err := support.RequireKata(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(s.Performance(context.Background())).To(Succeed())
}
