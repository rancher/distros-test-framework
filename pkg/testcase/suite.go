package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/factory"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	statusRunning = "Running"
	nslookup      = "kubernetes.default.svc.cluster.local"
)

// TestConfigVarVal tests that a given TF variable has the expected string value
func TestConfigVarVal(g GinkgoTInterface, key, expectedVal string) {
	actualVal := factory.GetConfigVarValue(g, key)
	Expect(actualVal).To(ContainSubstring(expectedVal),
		fmt.Sprintf("Wrong value passed in tfvars for '%s'", key))
}

// TestConfigVarSet tests that a given TF variable has the expected string value
func TestConfigVarSet(g GinkgoTInterface, key string) {
	actualVal := factory.GetConfigVarValue(g, key)
	Expect(actualVal).NotTo(BeEmpty(), fmt.Sprintf("Need to pass a value in tfvars for '%s'", key))
}
