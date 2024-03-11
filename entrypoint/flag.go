package entrypoint

import (
	"flag"
	"fmt"
	"os"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/template"
	"github.com/rancher/distros-test-framework/shared"
)

type flagAction func(name string)

func AddFlags(flagNames ...string) {
	if len(flagNames) == 0 {
		fmt.Println("No flags provided")
		os.Exit(1)
	}

	for _, name := range flagNames {
		if action, exists := flagActions[name]; exists {
			action(name)
		} else {
			fmt.Println("Invalid flag name provided")
		}
	}

	flag.Parse()
}

var flagActions = map[string]flagAction{
	"cmd": func(name string) {
		flag.StringVar(&template.TestMapTemplate.Cmd, name, "", "Comma separated list of commands to execute")
	},
	"expectedValue": func(name string) {
		flag.StringVar(&template.TestMapTemplate.ExpectedValue, name, "", "Comma separated list of expected values for commands")
	},
	"expectedValueUpgrade": func(name string) {
		flag.StringVar(&template.TestMapTemplate.ExpectedValueUpgrade, name, "", "Expected value of the command ran after upgrading")
	},
	"installVersionOrCommit": func(name string) {
		flag.Var(&customflag.ServiceFlag.InstallMode, name, "Upgrade with version or commit")
	},
	"channel": func(name string) {
		flag.Var(&customflag.ServiceFlag.Channel, name, "channel to use on install or upgrade")
	},
	"testCase": func(name string) {
		flag.Var(&customflag.TestCaseNameFlag, name, "Comma separated list of test case names to run")
	},
	"workloadName": func(name string) {
		flag.StringVar(&customflag.ServiceFlag.TestConfig.WorkloadName, name, "", "Name of the workload to a standalone deploy")
	},
	"applyWorkload": func(name string) {
		flag.BoolVar(&customflag.ServiceFlag.TestConfig.ApplyWorkload, name, false, "Deploy workload customflag for tests passed in")
	},
	"deleteWorkload": func(name string) {
		flag.BoolVar(&customflag.ServiceFlag.TestConfig.DeleteWorkload, name, false, "Delete workload customflag for tests passed in")
	},
	"destroy": func(name string) {
		flag.Var(&customflag.ServiceFlag.ClusterConfig.Destroy, name, "Destroy cluster after test")
	},
	"description": func(name string) {
		flag.StringVar(&customflag.ServiceFlag.TestConfig.Description, name, "", "Description of the test")
	},
	"sonobuoyVersion": func(name string) {
		flag.StringVar(&customflag.ServiceFlag.ExternalFlag.SonobuoyVersion, name, "0.56.17", "Sonobuoy Version that will be executed on the cluster")
	},
	"sucUpgradeVersion": func(name string) {
		flag.Var(&customflag.ServiceFlag.SUCUpgradeVersion, name, "Version for upgrading using SUC")
	},
	"certManagerVersion": func(name string) {
		flag.StringVar(&customflag.ServiceFlag.ExternalFlag.CertManagerVersion, name, "v1.11.0", "cert-manager version that will be deployed on the cluster")
	},
	"rancherHelmVersion": func(name string) {
		flag.StringVar(&customflag.ServiceFlag.ExternalFlag.RancherHelmVersion, name, "v2.8.0", "rancher helm chart version to use to deploy rancher manager")
	},
	"rancherImageVersion": func(name string) {
		flag.StringVar(&customflag.ServiceFlag.ExternalFlag.RancherImageVersion, name, "v2.8.0", "rancher version that will be deployed on the cluster")
	},
}

func ValidateInstallFlag() {
	if customflag.ServiceFlag.InstallMode.String() != "" && template.TestMapTemplate.ExpectedValueUpgrade == "" {
		shared.LogLevel("error", "if you are using upgrade, please provide the expected value after upgrade")
		os.Exit(1)
	}
}
