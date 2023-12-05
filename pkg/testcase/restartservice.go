package testcase

import (
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRestartService() {
	product, err := shared.GetProduct()
	Expect(err).NotTo(HaveOccurred(), "failed to get product")
	c := factory.ClusterConfig(GinkgoT())

	var ip string
	ip, err = shared.ManageService(product, "restart", "server", c.ServerIPs)
	Expect(err).NotTo(HaveOccurred(), "failed to get systemctl command for ip %s", ip)

	ip, err = shared.ManageService(product, "restart", "agent", c.AgentIPs)
	Expect(err).NotTo(HaveOccurred(), "failed to get systemctl command for ip %s", ip)
}
