package rancherday1

import (
	"context"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/entrypoint"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/rancher"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	cluster         *driver.Cluster
	infraConfig     *driver.InfraConfig
	flags           *customflag.FlagConfig
	cfg             *config.Env
	day1            *testcase.RancherDay1
	reportSummary   string
	reportErr       error
	err             error
	kdmSource       string
	version         string
	previousVersion string
	customNodes     string
	importNodes     string
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&flags.CertManager.Version, "certManagerVersion", "v1.16.1", "cert-manager version")
	flag.StringVar(&flags.Charts.Version, "chartsVersion", "", "rancher helm chart version")
	flag.StringVar(&flags.Charts.RepoName, "chartsRepoName", "", "rancher helm chart repo name")
	flag.StringVar(&flags.Charts.RepoUrl, "chartsRepoUrl", "", "rancher helm chart repo url")
	flag.StringVar(&flags.Charts.Args, "chartsArgs", "", "rancher helm additional args, comma separated")
	flag.StringVar(&kdmSource, "kdmSource", "", "KDM data.json https URL or kontainer-driver-metadata PR URL")
	flag.StringVar(&version, "k8sVersion", "", "version under test: custom cluster version and imported upgrade target")
	flag.StringVar(&previousVersion, "k8sPreviousVersion", "", "released version the imported cluster starts on")
	flag.StringVar(&customNodes, "customNodes", rancher.DefaultCustomLayout,
		"custom cluster nodes as roles:count groups, roles joined by + (etcd, controlplane|cp, worker)")
	flag.StringVar(&importNodes, "importNodes", rancher.DefaultImportLayout,
		"imported cluster nodes: etcd+controlplane:N servers, then worker:N agents")
	flag.Parse()

	customflag.ValidateVersionFormat()
	customflag.ValidateRancherChartsFlags()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	// Check the layouts and data.json before paying for infrastructure.
	day1Cfg := testcase.RancherDay1Config{
		Product: cfg.Product, Version: version,
		PreviousVersion: previousVersion, ResourceName: cfg.ResourceName,
	}
	day1Cfg.CustomNodes, day1Cfg.ImportNodes = parseLayouts()
	day1Cfg.DataURL = validateDay1(cfg.Product)
	entrypoint.ValidateRancherDeploy(cfg, flags)
	cluster, infraConfig = entrypoint.SetupClusterInfra(cfg)
	day1 = testcase.NewRancherDay1(cluster, &day1Cfg)
	os.Exit(m.Run())
}

func TestRancherDay1Suite(t *testing.T) {
	RegisterFailHandler(entrypoint.FailWithReport)
	RunSpecs(t, "Rancher Day-1 Operations Test Suite")
}

func parseLayouts() (custom, imported [][]string) {
	custom, err := rancher.ParseLayout(customNodes)
	if err == nil {
		err = rancher.ValidateCustomLayout(custom)
	}
	if err != nil {
		resources.LogLevel("error", "customNodes: %v\n", err)
		os.Exit(1)
	}

	imported, err = rancher.ParseLayout(importNodes)
	if err == nil {
		err = rancher.ValidateImportLayout(imported)
	}
	if err != nil {
		resources.LogLevel("error", "importNodes: %v\n", err)
		os.Exit(1)
	}
	resources.LogLevel("info", "Custom cluster nodes %q, imported cluster nodes %q", customNodes, importNodes)

	return custom, imported
}

func validateDay1(product string) string {
	if kdmSource == "" || version == "" || previousVersion == "" {
		resources.LogLevel("error", "kdmSource, k8sVersion and k8sPreviousVersion are required\n")
		os.Exit(1)
	}
	if previousVersion == version || strings.Contains(previousVersion, "-rc") {
		resources.LogLevel("error", "k8sPreviousVersion must be a released version older than k8sVersion\n")
		os.Exit(1)
	}
	for _, v := range []string{version, previousVersion} {
		if verr := rancher.ValidateVersion(product, v); verr != nil {
			resources.LogLevel("error", "%v\n", verr)
			os.Exit(1)
		}
	}

	ctx := context.Background()
	dataURL, rerr := rancher.ResolveDataURL(ctx, rancher.GitHubAPI, kdmSource)
	if rerr != nil {
		resources.LogLevel("error", "%v\n", rerr)
		os.Exit(1)
	}
	releases, ferr := rancher.FetchReleases(ctx, dataURL, product)
	if ferr != nil {
		resources.LogLevel("error", "%v\n", ferr)
		os.Exit(1)
	}
	for _, v := range []string{version, previousVersion} {
		rel, ok := rancher.FindRelease(releases, v)
		if !ok {
			resources.LogLevel("error", "%s is not in %s\n", v, dataURL)
			os.Exit(1)
		}
		resources.LogLevel("info", "KDM %s: rancher %s..%s", v, rel.MinChannelServerVersion, rel.MaxChannelServerVersion)
	}
	resources.LogLevel("info", "Using KDM data %s", dataURL)

	return dataURL
}

var _ = ReportAfterSuite("Rancher Day-1 Operations Test Suite",
	entrypoint.ReportAfterSuite(&cluster, &reportSummary))

var _ = AfterSuite(func() {
	day1.Cleanup(bool(flags.Destroy))
	entrypoint.AfterSuite(&cluster, &infraConfig, &reportSummary, &reportErr)()
})
