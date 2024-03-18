package assert

import (
	"net"
	"strings"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

// ValidateClusterIPsBySVC retrieves cluster IPs by svc and validates them in CIDR Range
func ValidateClusterIPsBySVC(svc string, expected []string) {
	cmd := "kubectl get svc " + svc +
		` -o jsonpath='{.spec.clusterIPs[*]}' --kubeconfig=` + factory.KubeConfigFile
	res, _ := shared.RunCommandHost(cmd)
	clusterIPs := strings.Split(res, " ")
	Expect(len(clusterIPs)).ShouldNot(BeZero())
	Expect(len(expected)).ShouldNot(BeZero())
	for i, ip := range clusterIPs {
		_, subnet, _ := net.ParseCIDR(expected[i])
		Expect(subnet.Contains(net.ParseIP(ip))).To(BeTrue())
	}
}
