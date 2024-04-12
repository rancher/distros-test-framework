package customflag

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rancher/distros-test-framework/shared"
)

var ServiceFlag FlagConfig
var TestCaseNameFlag stringSlice

// FlagConfig is a type that wraps all the flags that can be used
type FlagConfig struct {
	InstallMode       installModeFlag
	TestConfig        testConfigFlag
	ClusterConfig     clusterConfigFlag
	SUCUpgradeVersion sucUpgradeVersion
	Channel           channelFlag
	ExternalFlag      externalConfigFlag
}

type sucUpgradeVersion struct {
	SucUpgradeVersion string
}

type installModeFlag struct {
	Version string
	Commit  string
}

type channelFlag struct {
	Channel string
}

type testConfigFlag struct {
	TestFuncNames  []string
	TestFuncs      []TestCaseFlag
	ApplyWorkload  bool
	DeleteWorkload bool
	WorkloadName   string
	Description    string
	DebugMode      bool
}

type externalConfigFlag struct {
	SonobuoyVersion    string
	CertManagerVersion string
	HelmChartsFlag     helmChartsFlag
	RancherVersion     string
}

type helmChartsFlag struct {
	Args     string
	Version  string
	RepoName string
	RepoUrl  string
}

type clusterConfigFlag struct {
	Destroy destroyFlag
}

type destroyFlag bool

type TestCaseFlag func(applyWorkload, deleteWorkload bool)

type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(value string) error {
	*s = strings.Split(value, ",")

	return nil
}

func (t *testConfigFlag) String() string {
	return fmt.Sprintf("TestFuncName: %s", t.TestFuncNames)
}

func (t *testConfigFlag) Set(value string) error {
	t.TestFuncNames = strings.Split(value, ",")

	return nil

}
func (c *channelFlag) String() string {
	return c.Channel
}

func (c *channelFlag) Set(value string) error {
	if value == "" {
		return nil
	}

	if value != "latest" && value != "stable" && value != "testing" {
		return shared.ReturnLogError("invalid channel: %s", value)
	}

	c.Channel = value

	return nil
}

func (i *installModeFlag) String() string {
	return fmt.Sprintf("%s%s", i.Version, i.Commit)
}

func (i *installModeFlag) Set(value string) error {
	if strings.HasPrefix(value, "v") {
		if !strings.Contains(value, "k3s") && !strings.Contains(value, "rke2") {
			return shared.ReturnLogError("invalid version format: %s", value)
		}
		i.Version = value
	} else {
		if len(value) != 40 {
			return shared.ReturnLogError("invalid commit length: %s", value)
		}
		i.Commit = value
	}

	return nil
}

func (t *sucUpgradeVersion) String() string {
	return t.SucUpgradeVersion
}

func (t *sucUpgradeVersion) Set(value string) error {
	if !strings.HasPrefix(value, "v") ||
		(!strings.Contains(value, "k3s") && !strings.Contains(value, "rke2")) {
		return shared.ReturnLogError("suc upgrade only accepts version format: %s", value)
	}
	t.SucUpgradeVersion = value

	return nil
}

func (d *destroyFlag) String() string {
	return fmt.Sprintf("%v", *d)
}

func (d *destroyFlag) Set(value string) error {
	v, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	*d = destroyFlag(v)

	return nil
}

func (e *externalConfigFlag) String() string {
	return e.SonobuoyVersion
}

func (e *externalConfigFlag) Set(value string) error {
	e.SonobuoyVersion = value

	return nil
}
