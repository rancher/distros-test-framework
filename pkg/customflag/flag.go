package customflag

import (
	"flag"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/shared"
)

type flagAction func(name string)

func AddFlags(flagNames ...string) {
	if len(flagNames) == 0 {
		shared.LogLevel("error", "no flag names provided")
		os.Exit(1)
	}

	for _, name := range flagNames {
		if action, exists := flagActions[name]; exists {
			action(name)
		} else {
			shared.LogLevel("error", "flag name %s does not exist", name)
			os.Exit(1)
		}
	}

	flag.Parse()
}

var flagActions = map[string]flagAction{
	"cmd": func(name string) {
		flag.StringVar(&Tm.Cmd, name, "", "Comma separated list of commands to execute")
	},
	"expectedValue": func(name string) {
		flag.StringVar(&Tm.ExpectedValue, name, "", "Comma separated list of expected values for commands")
	},
	"expectedValueUpgrade": func(name string) {
		flag.StringVar(&Tm.ExpectedValueUpgrade, name, "", "Expected value of the command ran after upgrading")
	},
	"installVersionOrCommit": func(name string) {
		flag.Var(&ServiceFlag.InstallMode, name, "Upgrade with version or commit")
	},
	"channel": func(name string) {
		flag.Var(&ServiceFlag.Channel, name, "channel to use on install or upgrade")
	},
	"testCase": func(name string) {
		flag.Var(&TestCaseNameFlag, name, "Comma separated list of test case names to run")
	},
	"workloadName": func(name string) {
		flag.StringVar(&ServiceFlag.TestConfig.WorkloadName, name, "", "Name of the workload to a standalone deploy")
	},
	"applyWorkload": func(name string) {
		flag.BoolVar(&ServiceFlag.TestConfig.ApplyWorkload, name, false, "Deploy workload customflag for tests passed in")
	},
	"deleteWorkload": func(name string) {
		flag.BoolVar(&ServiceFlag.TestConfig.DeleteWorkload, name, false, "Delete workload customflag for tests passed in")
	},
	"destroy": func(name string) {
		flag.Var(&ServiceFlag.ClusterConfig.Destroy, name, "Destroy cluster after test")
	},
	"description": func(name string) {
		flag.StringVar(&ServiceFlag.TestConfig.Description, name, "", "Description of the test")
	},
	"sonobuoyVersion": func(name string) {
		flag.StringVar(&ServiceFlag.ExternalFlag.SonobuoyVersion, name, "0.56.17", "Sonobuoy Version that will be executed on the cluster")
	},
	"sucUpgradeVersion": func(name string) {
		flag.Var(&ServiceFlag.SUCUpgradeVersion, name, "Version for upgrading using SUC")
	},
	"certManagerVersion": func(name string) {
		flag.StringVar(&ServiceFlag.ExternalFlag.CertManagerVersion, name, "v1.11.0", "cert-manager version that will be deployed on the cluster")
	},
	"rancherHelmVersion": func(name string) {
		flag.StringVar(&ServiceFlag.ExternalFlag.RancherHelmVersion, name, "v2.8.0", "rancher helm chart version to use to deploy rancher manager")
	},
	"rancherImageVersion": func(name string) {
		flag.StringVar(&ServiceFlag.ExternalFlag.RancherImageVersion, name, "v2.8.0", "rancher version that will be deployed on the cluster")
	},
}

// ValidateFlags validates the flags that were set
func ValidateFlags() {
	if Tm.Cmd == "" || Tm.ExpectedValue == "" {
		shared.LogLevel("error", "error: command and/or expected value was not sent")
		os.Exit(1)
	}

	cmds := strings.Split(Tm.Cmd, ",")
	expectedValues := strings.Split(Tm.ExpectedValue, ",")

	if len(cmds) != len(expectedValues) {
		shared.LogLevel("error", "mismatched length commands x expected values: %s x %s", cmds, expectedValues)
		os.Exit(1)
	}

	validateInstallFlag()
}

// validateInstallFlag validates if the install flag is set then should always have a value in the upgrade flag
func validateInstallFlag() {
	if ServiceFlag.InstallMode.String() != "" && Tm.ExpectedValueUpgrade == "" {
		shared.LogLevel("error", "using upgrade, please provide the expected value after upgrade")
		os.Exit(1)
	}
}
