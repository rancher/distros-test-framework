package rancherday1

import (
	"fmt"

	"github.com/rancher/distros-test-framework/internal/pkg/assert"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase"

	. "github.com/onsi/ginkgo/v2"
)

// ContinueOnFailure keeps independent steps (e.g. import) running; dependent steps Skip on missing prerequisites.
var _ = Describe("Test Rancher Day-1 Operations:", Ordered, ContinueOnFailure, func() {
	It("Start Up with no issues", func() {
		testcase.TestBuildCluster(cluster)
	})

	It("Validate Nodes", func() {
		testcase.TestNodeStatus(cluster, assert.NodeAssertReadyStatus(), nil)
	})

	It("Validate Pods", func() {
		testcase.TestPodStatus(cluster, assert.PodAssertRestart(), assert.PodAssertReady())
	})

	It("Deploys cert-manager", func() {
		testcase.TestDeployCertManager(cluster, flags.CertManager.Version)
	})

	It("Deploys rancher manager", func() {
		testcase.TestDeployRancher(cluster, flags)
	})

	It("Connects to the Rancher API and UI", func() { day1.ConnectRancher() })

	It("Loads the KDM data.json under test", func() { day1.LoadKDM() })

	It("Lists the version under test in the create-cluster dropdown", func() { day1.CheckCreateDropdown() })

	It("Provisions a custom cluster", func() { day1.ProvisionCustom() })

	It("Takes an etcd snapshot of the custom cluster", func() { day1.SnapshotCustom() })

	It("Restores the etcd snapshot on the custom cluster", func() { day1.RestoreCustom() })

	It("Imports a standalone cluster on the previous release", func() { day1.ImportStandalone() })

	It("Lists the version under test in the imported cluster's edit dropdown", func() {
		day1.CheckImportUpgradeDropdown()
	})

	It("Upgrades the imported cluster to the version under test", func() { day1.UpgradeImported() })
})

var _ = AfterEach(func() {
	report := CurrentSpecReport()
	if report.Failed() {
		fmt.Printf("\nFAILED! %s\n\n", report.FullText())
		day1.CollectDebug(report.LeafNodeText)
	} else {
		fmt.Printf("\nPASSED! %s\n\n", report.FullText())
	}
})
