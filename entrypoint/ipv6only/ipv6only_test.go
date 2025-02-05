package ipv6only

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Test ipv6 only cluster:", func() {
	It("Start Up with no issues", func() {
		testcase.TestBuildIPv6OnlyCluster(cluster)
	})

	It("Install product on ipv6 only nodes", func() {
		testcase.TestIPv6Only(cluster, awsClient)
	})

	// TODO: validates

})

var _ = AfterEach(func() {
	if CurrentSpecReport().Failed() {
		fmt.Printf("\nFAILED! %s\n\n", CurrentSpecReport().FullText())
	} else {
		fmt.Printf("\nPASSED! %s\n\n", CurrentSpecReport().FullText())
	}
})
