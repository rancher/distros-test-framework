package versionbump

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/template"
	"github.com/rancher/distros-test-framework/internal/provisioning"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/provisioning/legacy"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	cluster     *driver.Cluster
	cfg         *config.Env
	infraConfig *driver.InfraConfig
	err         error
)

func TestMain(m *testing.M) {
	flag.StringVar(&customflag.TestMap.Cmd, "cmd", "", "Comma separated list of commands to execute")
	flag.StringVar(&customflag.TestMap.ExpectedValue, "expectedValue", "", "Comma separated list of expected values for commands")
	flag.StringVar(&customflag.TestMap.ExpectedValueUpgrade, "expectedValueUpgrade", "", "Expected value of the command ran after upgrading")
	flag.Var(&customflag.ServiceFlag.InstallMode, "installVersionOrCommit", "Upgrade with version or commit")
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

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	customflag.ValidateTemplateFlags()

	setupClusterInfra()

	os.Exit(m.Run())
}

func TestVersionBumpSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Version Test Suite")
}

func setupClusterInfra() {
	kubeconfig := os.Getenv("KUBE_CONFIG")
	if kubeconfig != "" {
		// gets a cluster from existing kubeconfig.
		cluster = legacy.KubeConfigCluster(kubeconfig)
		resources.LogLevel("info", "Using existing cluster from kubeconfig")

		return
	}

	// initial data load needed for provisioning comming from config env vars.
	infraConfig = &driver.InfraConfig{
		Product:           cfg.Product,
		Module:            cfg.Module,
		ResourceName:      cfg.ResourceName,
		ProvisionerModule: cfg.ProvisionerModule,
		ProvisionerType:   cfg.ProvisionerType,
		InstallVersion:    cfg.InstallVersion,
		QAInfraProvider:   cfg.QAInfraProvider,
		NodeOS:            cfg.NodeOS,
		CNI:               cfg.CNI,
		Cluster: &driver.Cluster{
			Config: driver.Config{
				Arch:        cfg.Arch,
				ServerFlags: cfg.ServerFlags,
				WorkerFlags: cfg.WorkerFlags,
				Channel:     cfg.Channel,
			},
			SSH: driver.SSHConfig{
				User:        cfg.SSHUser,
				PrivKeyPath: cfg.SSHKeyPath,
				KeyName:     cfg.SSHKeyName,
			},
		},
	}

	cluster, err = provisioning.ProvisionInfrastructure(infraConfig)
	if err != nil {
		resources.LogLevel("error", "error provisioning infrastructure: %w\n", err)
		os.Exit(1)
	}

	resources.LogLevel("info", "Cluster provisioned successfully with %+v", cluster)
}

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		status, err := provisioning.DestroyInfrastructure(
			infraConfig.ProvisionerModule, infraConfig.Product, infraConfig.Module)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}

	testTag := os.Getenv("TEST_TAG")
	if testTag == "components" {
		template.ComponentsBumpResults()
	}
	if testTag != "versionbump" {
		resources.PrintGetAll()
	}
})

func addTcFlag() {
	customflag.ValidateTemplateTcs()

	testFuncs, err := template.AddTestCases(cluster, customflag.ServiceFlag.TestTemplateConfig.TestFuncNames)
	if err != nil {
		resources.LogLevel("error", "error on adding test cases to testConfigFlag: %w", err)
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
