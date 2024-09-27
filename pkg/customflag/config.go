package customflag

import (
	"fmt"
	"strconv"
	"strings"
)

var (
	ServiceFlag      FlagConfig
	TestCaseNameFlag stringSlice
	TestMap          testMapConfigFlag
)

// FlagConfig is a type that wraps all the flags that can be used.
type FlagConfig struct {
	InstallMode        installModeFlag
	TestTemplateConfig templateConfigFlag
	Destroy            destroyFlag
	SUCUpgradeVersion  sucUpgradeVersionFlag
	Channel            channelFlag
	External           externalConfigFlag
	RancherConfig      rancherConfigFlag
	HelmCharts         helmChartsFlag
	S3Flags            s3ConfigFlag
}

// TestMapConfig is a type that wraps the test commands and expected values.
type TestMapConfig testMapConfigFlag

// testMapConfigFlag represents a single test command with key:value pairs.
type testMapConfigFlag struct {
	Cmd                  string
	ExpectedValue        string
	ExpectedValueUpgrade string
}

type TestCaseFlag func(applyWorkload, deleteWorkload bool)

type templateConfigFlag struct {
	TestFuncNames  []string
	TestFuncs      []TestCaseFlag
	ApplyWorkload  bool
	DeleteWorkload bool
	WorkloadName   string
	Description    string
}

func (t *templateConfigFlag) String() string {
	return fmt.Sprintf("TestFuncName: %s", t.TestFuncNames)
}

func (t *templateConfigFlag) Set(value string) error {
	t.TestFuncNames = strings.Split(value, ",")

	return nil
}

type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(value string) error {
	*s = strings.Split(value, ",")

	return nil
}

type channelFlag struct {
	Channel string
}

func (c *channelFlag) String() string {
	return c.Channel
}

func (c *channelFlag) Set(value string) error {
	if value == "" {
		return nil
	}

	if value != "latest" && value != "stable" && value != "testing" {
		return fmt.Errorf("invalid channel: %s", value)
	}

	c.Channel = value

	return nil
}

type installModeFlag struct {
	Version string
	Commit  string
}

func (i *installModeFlag) String() string {
	return fmt.Sprintf("%s%s", i.Version, i.Commit)
}

func (i *installModeFlag) Set(value string) error {
	if strings.HasPrefix(value, "v") {
		if !strings.Contains(value, "k3s") && !strings.Contains(value, "rke2") {
			return fmt.Errorf("invalid version format: %s", value)
		}
		i.Version = value
	} else {
		if len(value) != 40 {
			return fmt.Errorf("invalid commit length: %s", value)
		}
		i.Commit = value
	}

	return nil
}

type sucUpgradeVersionFlag struct {
	SUCUpgradeVersion string
}

func (t *sucUpgradeVersionFlag) String() string {
	return t.SUCUpgradeVersion
}

func (t *sucUpgradeVersionFlag) Set(value string) error {
	if !strings.HasPrefix(value, "v") || (!strings.Contains(value, "k3s") && !strings.Contains(value, "rke2")) {
		return fmt.Errorf("suc upgrade only accepts version format: %s", value)
	}

	t.SUCUpgradeVersion = value

	return nil
}

type destroyFlag bool

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

type externalConfigFlag struct {
	SonobuoyVersion string
}

func (e *externalConfigFlag) String() string {
	return e.SonobuoyVersion
}

func (e *externalConfigFlag) Set(value string) error {
	e.SonobuoyVersion = value

	return nil
}

type helmChartsFlag struct {
	Args     string
	Version  string
	RepoName string
	RepoUrl  string
}

type rancherConfigFlag struct {
	CertManagerVersion string
	RancherVersion     string
}

type s3ConfigFlag struct {
	Bucket string
	Folder string
}
