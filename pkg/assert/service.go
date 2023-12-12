package assert

import (
	"net"
	"strings"

	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

// CheckServiceSpecContainsSubstring retrives service specs and validates expected substring
func CheckServiceSpecContainsSubstring(svc, expected string) {
	cmd := "kubectl get svc "+svc+" -n default"+
		" -o jsonpath='{range .items[*]}{.spec}' --kubeconfig=" + shared.KubeConfigFile
	res, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), err)
	Expect(res).To(ContainSubstring(expected))
}

// ValidateClusterIPsBySVC retrives cluster IPs by svc and validates them in CIDR Range  
func ValidateClusterIPsBySVC(svc string, expected []string) {
	cmd := "kubectl get svc "+svc+
		` -o jsonpath='{.spec.clusterIPs[*]}' --kubeconfig=` + shared.KubeConfigFile
	Eventually(func() error {
		res, _ := shared.RunCommandHost(cmd)
		ips := strings.Split(res, " ")
		Expect(len(ips)).ShouldNot(BeZero())
		Expect(len(expected)).ShouldNot(BeZero())
		for i, ip := range ips {
			_,subnet,_ := net.ParseCIDR(expected[i])
			if subnet.Contains(net.ParseIP(ip)) {
				return nil
			}
		}
		return nil
	}, "180s", "5s").Should(Succeed(), 
	"failed to validate clusterIPs in expected range %s for svc  %s", 
	expected, svc)
}