package testcase

import (
	"context"

	"github.com/rancher/distros-test-framework/internal/pkg/testcase/support"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"

	. "github.com/onsi/gomega"
)

func TestKataInstall(cluster *driver.Cluster, cfg *support.KataConfig) {
	Expect(support.StartKata(context.Background(), cluster, cfg)).To(Succeed())
}

func TestKataRuntime(cfg *support.KataConfig) {
	s, err := support.RequireKata(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(s.RuntimeIdentity(context.Background())).To(Succeed())
}

func TestKataCoexistence(cfg *support.KataConfig) {
	s, err := support.RequireKata(cfg, "kata-local", "runc-local", "runc-remote")
	Expect(err).NotTo(HaveOccurred())
	Expect(s.Coexistence(context.Background())).To(Succeed())
}

func TestKataRejectIneligible(cfg *support.KataConfig) {
	s, err := support.RequireKata(cfg, "runc-remote")
	Expect(err).NotTo(HaveOccurred())
	Expect(s.RejectIneligible(context.Background())).To(Succeed())
}

func TestKataRestart(cfg *support.KataConfig) {
	s, err := support.RequireKata(cfg, "kata-local", "runc-local", "runc-remote")
	Expect(err).NotTo(HaveOccurred())
	Expect(s.Restart(context.Background())).To(Succeed())
}

func TestKataIsolation(cfg *support.KataConfig) {
	s, err := support.RequireKata(cfg, "kata-local")
	Expect(err).NotTo(HaveOccurred())
	Expect(s.Isolation(context.Background())).To(Succeed())
}

func TestKataPerformance(cfg *support.KataConfig) {
	s, err := support.RequireKata(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(s.Performance(context.Background())).To(Succeed())
}
