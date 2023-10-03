package assert

import (
	"net"
	"strings"

	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func CheckServiceSpecContainsSubstring(svc, expected string) {
	// cmd := "kubectl get svc "+svc+" -n default"+
	// 	" -o jsonpath='{range .items[*]}{.spec}' --kubeconfig=" + shared.KubeConfigFile

	res, err := shared.KubectlCommand(
		"host",
		"get",
		"svc",
		svc,
		"-n default",
		"-o jsonpath='{range .items[*]}{.spec}' --kubeconfig=",
		shared.KubeConfigFile,
	)
	//res, err := shared.RunCommandHost(cmd)
	Expect(err).NotTo(HaveOccurred(), err)
	Expect(res).To(ContainSubstring(expected))
}

// ValidateClusterIPsBySVC validates expected pod IPs by svc 
func ValidateClusterIPsBySVC(svc string, expected []string) {
	Eventually(func() error {
		res, _ := shared.KubectlCommand(
			"host",
			"get",
			"svc "+ svc +` -o jsonpath='{.spec.clusterIPs[*]}'`,
		)
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