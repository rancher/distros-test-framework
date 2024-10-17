package template

import (
	"strings"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/k8s"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func Template(template TestTemplate) {
	if customflag.ServiceFlag.TestTemplateConfig.WorkloadName != "" &&
		strings.HasSuffix(customflag.ServiceFlag.TestTemplateConfig.WorkloadName, ".yaml") {
		err := shared.ManageWorkload(
			"apply",
			customflag.ServiceFlag.TestTemplateConfig.WorkloadName,
		)
		Expect(err).NotTo(HaveOccurred())
	}

	err := executeTestCombination(template)
	Expect(err).NotTo(HaveOccurred(), "error validating test template: %w", err)

	k8sClient, err := k8s.Add()
	Expect(err).NotTo(HaveOccurred(), "error adding k8s: %w", err)

	if template.InstallMode != "" {
		upgErr := upgradeVersion(template, k8sClient, template.InstallMode)
		Expect(upgErr).NotTo(HaveOccurred(), "error upgrading version: %w", upgErr)

		err = executeTestCombination(template)
		Expect(err).NotTo(HaveOccurred(), "error validating test template: %w", err)

		if template.TestConfig != nil {
			testCaseWrapper(template)
		}
	}
}
