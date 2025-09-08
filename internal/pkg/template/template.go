package template

import (
	"strings"

	. "github.com/onsi/gomega"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/k8s"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

func Template(cluster *driver.Cluster, template TestTemplate) {
	if customflag.ServiceFlag.TestTemplateConfig.WorkloadName != "" &&
		strings.HasSuffix(customflag.ServiceFlag.TestTemplateConfig.WorkloadName, ".yaml") {
		err := resources.ManageWorkload(
			"apply",
			customflag.ServiceFlag.TestTemplateConfig.WorkloadName,
		)
		Expect(err).NotTo(HaveOccurred())
	}

	err := executeTestCombination(template)
	Expect(err).NotTo(HaveOccurred(), "error validating test template: %w", err)

	k8sClient, err := k8s.AddClient()
	Expect(err).NotTo(HaveOccurred(), "error adding k8s: %w", err)

	if template.InstallMode != "" {
		upgErr := upgradeVersion(cluster, template, k8sClient, template.InstallMode)
		Expect(upgErr).NotTo(HaveOccurred(), "error upgrading version: %w", upgErr)

		err = executeTestCombination(template)
		Expect(err).NotTo(HaveOccurred(), "error validating test template: %w", err)

		if template.TestConfig != nil {
			testCaseWrapper(template)
		}
	}
}
