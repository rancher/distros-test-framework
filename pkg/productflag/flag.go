package productflag

import (
	"flag"
	"os"
	"regexp"
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

	flag.Visit(func(f *flag.Flag) {
		shared.LogLevel("info", "added flag:\n%s value: %s", f.Name, f.Value)
	})
}

var flagActions = map[string]flagAction{
	"cmd": func(name string) {
		flag.StringVar(&TestMap.Cmd, name, "", "Comma separated list of commands to execute")
	},
	"expectedValue": func(name string) {
		flag.StringVar(&TestMap.ExpectedValue, name, "", "Comma separated list of expected values for commands")
	},
	"expectedValueUpgrade": func(name string) {
		flag.StringVar(&TestMap.ExpectedValueUpgrade, name, "", "Expected value of the command ran after upgrading")
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
		flag.StringVar(&ServiceFlag.TestTemplateConfig.WorkloadName, name, "", "Name of the workload to a standalone deploy")
	},
	"applyWorkload": func(name string) {
		flag.BoolVar(&ServiceFlag.TestTemplateConfig.ApplyWorkload, name, false, "Deploy workload productflag for tests passed in")
	},
	"deleteWorkload": func(name string) {
		flag.BoolVar(&ServiceFlag.TestTemplateConfig.DeleteWorkload, name, false, "Delete workload productflag for tests passed in")
	},
	"destroy": func(name string) {
		flag.Var(&ServiceFlag.Destroy, name, "Destroy cluster after test")
	},
	"description": func(name string) {
		flag.StringVar(&ServiceFlag.TestTemplateConfig.Description, name, "", "Description of the test")
	},
	"sonobuoyVersion": func(name string) {
		flag.StringVar(&ServiceFlag.External.SonobuoyVersion, name, "0.56.17", "Sonobuoy Version that will be executed on the cluster")
	},
	"sucUpgradeVersion": func(name string) {
		flag.Var(&ServiceFlag.SUCUpgradeVersion, name, "Version for upgrading using SUC")
	},
	"certManagerVersion": func(name string) {
		flag.StringVar(&ServiceFlag.RancherConfig.CertManagerVersion, name, "v1.13.1", "cert-manager version that will be deployed on the cluster")
	},
	"rancherHelmVersion": func(name string) {
		flag.StringVar(&ServiceFlag.RancherConfig.RancherHelmVersion, name, "v2.8.3", "rancher helm chart version to use to deploy rancher manager")
	},
	"rancherImageVersion": func(name string) {
		flag.StringVar(&ServiceFlag.RancherConfig.RancherImageVersion, name, "v2.8.3", "rancher version that will be deployed on the cluster")
	},
}

// ValidateTemplateFlags validates version bump template flags that were set.
func ValidateTemplateFlags() {
	if TestMap.Cmd == "" {
		shared.LogLevel("error", "cmd was not sent")
		os.Exit(1)
	}
	if TestMap.ExpectedValue == "" {
		shared.LogLevel("error", "expected value was not sent")
		os.Exit(1)
	}

	// for now we are validating that the length of commands and expected/upgraded values are the same.
	cmds := strings.Split(TestMap.Cmd, ",")
	expectedValues := strings.Split(TestMap.ExpectedValue, ",")
	if len(cmds) != len(expectedValues) {
		shared.LogLevel("error", "mismatched length commands x expected values: %s x %s", cmds, expectedValues)
		os.Exit(1)
	}
	if TestMap.ExpectedValueUpgrade != "" {
		expectedValuesUpgrade := strings.Split(TestMap.ExpectedValueUpgrade, ",")
		if len(cmds) != len(expectedValuesUpgrade) {
			shared.LogLevel("error", "mismatched length commands x expected values upgrade: %s x %s", cmds, expectedValuesUpgrade)
			os.Exit(1)
		}
	}

	if ServiceFlag.InstallMode.String() != "" && TestMap.ExpectedValueUpgrade == "" {
		shared.LogLevel("error", "using upgrade, please provide the expected value after upgrade")
		os.Exit(1)
	}
}

func ValidateVersionFormat() {
	rancherFlags := []string{
		ServiceFlag.RancherConfig.CertManagerVersion,
		ServiceFlag.RancherConfig.RancherHelmVersion,
		ServiceFlag.RancherConfig.RancherImageVersion,
	}

	re := regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	for _, v := range rancherFlags {
		if !re.MatchString(v) {
			shared.LogLevel("error", "invalid format: %s, expected format: v.xx.xx.xx", v)
			os.Exit(1)
		}
	}
}
