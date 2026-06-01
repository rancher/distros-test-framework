package entrypoint

import (
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/qase"
	"github.com/rancher/distros-test-framework/internal/provisioning"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/provisioning/legacy"
	"github.com/rancher/distros-test-framework/internal/report"
	"github.com/rancher/distros-test-framework/internal/resources"
)

func SetupClusterInfra(cfg *config.Env) (*driver.Cluster, *driver.InfraConfig) {
	if kubeconfig := os.Getenv("KUBE_CONFIG"); kubeconfig != "" {
		resources.LogLevel("info", "Using existing cluster from kubeconfig")
		return legacy.KubeConfigCluster(kubeconfig), nil
	}

	infraConfig := &driver.InfraConfig{
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

	cluster, err := provisioning.ProvisionInfrastructure(infraConfig)
	if err != nil {
		resources.LogLevel("error", "error provisioning infrastructure: %w\n", err)
		os.Exit(1)
	}
	resources.LogLevel("info", "Cluster provisioned successfully with %+v", cluster.Config)

	return cluster, infraConfig
}

func FailWithReport(message string, callerSkip ...int) {
	skip := 1
	if len(callerSkip) > 0 {
		skip = callerSkip[0] + 1
	}
	ginkgo.Fail(message, skip)
}

func CheckSelinuxTest(serverFlags string, selinuxFlagEnabled bool) {
	if !selinuxFlagEnabled {
		resources.LogLevel("info", "Skipping selinux test")
		return
	}
	if !strings.Contains(serverFlags, "selinux: true") {
		resources.LogLevel("error",
			"selinux test is enabled but SERVER_FLAGS does not contain 'selinux: true'")
		os.Exit(1)
	}
	resources.LogLevel("info", "Running selinux test")
}

// CheckIngressCompat aborts the suite early when SERVER_FLAGS pins.
// Allow: rke2 + >=1.36 + ingress-controller: nginx
// Reject: rke2 + <1.36  + ingress-controller: traefik  - INCOMPATIBLE.
func CheckIngressCompat(cfg *config.Env) {
	if cfg.Product != "rke2" {
		return
	}
	pinned := extractIngressControllerPin(cfg.ServerFlags)
	if pinned == "" {
		return
	}
	atLeast136 := isRKE2AtLeast(cfg.InstallVersion, 1, 36)

	if pinned == "traefik" && !atLeast136 {
		resources.LogLevel("error",
			"SERVER_FLAGS pins ingress-controller: traefik but INSTALL_VERSION=%q is "+
				"pre-1.36 (no bundled traefik chart). Drop the ingress-controller line "+
				"or bump INSTALL_VERSION to >= v1.36.x.", cfg.InstallVersion)
		os.Exit(1)
	}
	if (pinned == "nginx" && !atLeast136) || (pinned == "traefik" && atLeast136) {
		resources.LogLevel("error",
			"SERVER_FLAGS pins ingress-controller: %s but that's already the "+
				"RKE2 %s default. Remove the line — pinning is only meaningful "+
				"when overriding (e.g., ingress-controller: nginx on >=1.36 for "+
				"legacy compat).", pinned, cfg.InstallVersion)
		os.Exit(1)
	}
}

func extractIngressControllerPin(serverFlags string) string {
	for _, line := range strings.Split(serverFlags, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ingress-controller:") {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(line, "ingress-controller:"))
		val = strings.Trim(val, `"'`)

		return val
	}

	return ""
}

var rke2MinorVersionRE = regexp.MustCompile(`^v?(\d+)\.(\d+)`)

func isRKE2AtLeast(installVersion string, major, minor int) bool {
	m := rke2MinorVersionRE.FindStringSubmatch(installVersion)
	if len(m) < 3 {
		return false
	}
	maj, errMaj := strconv.Atoi(m[1])
	minVal, errMin := strconv.Atoi(m[2])
	if errMaj != nil || errMin != nil {
		return false
	}
	if maj != major {
		return maj > major
	}

	return minVal >= minor
}

func ReportAfterSuite(
	clusterPtr **driver.Cluster,
	summaryPtr *string,
) func(ginkgo.Report) {
	return func(specReport ginkgo.Report) {
		if !strings.EqualFold(os.Getenv("REPORT_TO_QASE"), "true") {
			resources.LogLevel("info", "Qase reporting is not enabled")
			return
		}
		client, err := qase.AddQase()
		if err != nil {
			resources.LogLevel("error", "error adding qase: %v", err)
			return
		}
		client.SpecReportTestResults(client.Ctx, *clusterPtr, &specReport, *summaryPtr)
	}
}

func AfterSuite(
	clusterPtr **driver.Cluster,
	infraConfigPtr **driver.InfraConfig,
	summaryPtr *string,
	errPtr *error, //nolint:gocritic // out-param written by AfterSuite
) func() {
	return func() {
		flags := &customflag.ServiceFlag
		*summaryPtr, *errPtr = report.SummaryReportData(*clusterPtr, flags)
		if *errPtr != nil {
			resources.LogLevel("error", "error getting report summary data: %v\n", *errPtr)
		}
		if !flags.Destroy {
			return
		}
		ic := *infraConfigPtr
		status, derr := provisioning.DestroyInfrastructure(
			ic.ProvisionerModule, ic.Product, ic.Module)
		gomega.Expect(derr).ToNot(gomega.HaveOccurred())
		gomega.Expect(status).To(gomega.Equal("cluster destroyed"))
	}
}

func DestroyOnlyAfterSuite(infraConfigPtr **driver.InfraConfig) func() {
	return func() {
		if !customflag.ServiceFlag.Destroy {
			return
		}
		ic := *infraConfigPtr
		status, derr := provisioning.DestroyInfrastructure(
			ic.ProvisionerModule, ic.Product, ic.Module)
		gomega.Expect(derr).ToNot(gomega.HaveOccurred())
		gomega.Expect(status).To(gomega.Equal("cluster destroyed"))
	}
}
