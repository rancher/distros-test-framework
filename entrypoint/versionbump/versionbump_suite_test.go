package versionbump

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	kubeconfig string
	cluster    *shared.Cluster
	awsClient  *aws.Client
)

func TestMain(m *testing.M) {
	flag.StringVar(&customflag.TestMap.Cmd, "cmd", "", "Comma separated list of commands to execute")
	flag.StringVar(&customflag.TestMap.ExpectedValue, "expectedValue", "", "Comma separated list of expected values for commands")
	flag.StringVar(&customflag.TestMap.ExpectedValueUpgrade, "expectedValueUpgrade", "", "Expected value of the command ran after upgrading")
	flag.Var(&customflag.ServiceFlag.UpgradeMode, "installVersionOrCommit", "Upgrade with version or commit")
	flag.Var(&customflag.ServiceFlag.Channel, "channel", "channel to use on install or upgrade")
	flag.Var(&customflag.TestCaseNameFlag, "testCase", "Comma separated list of test case names to run")
	flag.StringVar(&customflag.ServiceFlag.TestTemplateConfig.WorkloadName, "workloadName", "", "Name of the workload to a standalone deploy")
	flag.BoolVar(&customflag.ServiceFlag.TestTemplateConfig.ApplyWorkload, "applyWorkload", false, "Deploy workload customflag for tests passed in")
	flag.BoolVar(&customflag.ServiceFlag.TestTemplateConfig.DeleteWorkload, "deleteWorkload", false, "Delete workload customflag for tests passed in")
	flag.Var(&customflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&customflag.ServiceFlag.TestTemplateConfig.Description, "description", "", "Description of the test")
	flag.Parse()

	customflag.ServiceFlag.TestTemplateConfig.TestFuncNames = customflag.TestCaseNameFlag
	if customflag.ServiceFlag.TestTemplateConfig.TestFuncNames != nil {
		addTcFlag()
	}

	_, err := config.AddEnv()
	if err != nil {
		shared.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	customflag.ValidateTemplateFlags()

	kubeconfig = os.Getenv("KUBE_CONFIG")
	if kubeconfig == "" {
		// gets a cluster from terraform.
		cluster = shared.ClusterConfig()
	} else {
		// gets a cluster from kubeconfig.
		cluster = shared.KubeConfigCluster(kubeconfig)
	}

	awsClient, err = aws.AddClient(cluster)
	if err != nil {
		shared.LogLevel("error", "error adding aws client: %w\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestVersionBumpSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Version Test Suite")
}

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		status, err := shared.DestroyCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}

	testTag := os.Getenv("TEST_TAG")
	if testTag == "components" {
		template.ComponentsBumpResults()
	}
	if testTag != "versionbump" {
		shared.PrintGetAll()
	}
})

func addTcFlag() {
	customflag.ValidateTemplateTcs()

	testFuncs, err := template.AddTestCases(cluster, awsClient, customflag.ServiceFlag.TestTemplateConfig.TestFuncNames)
	if err != nil {
		shared.LogLevel("error", "error on adding test cases to testConfigFlag: %w", err)
		return
	}
	if len(testFuncs) > 0 {
		testCaseFlags := make([]customflag.TestCaseFlag, len(testFuncs))
		for i, j := range testFuncs {
			testCaseFlags[i] = customflag.TestCaseFlag(j)
		}
		customflag.ServiceFlag.TestTemplateConfig.TestFuncs = testCaseFlags
	}
}
