package customflag

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rancher/distros-test-framework/shared"
)

var ServiceFlag FlagConfig
var TestCaseNameFlag StringSlice

// FlagConfig is a type that wraps all the flags that can be used
type FlagConfig struct {
	InstallMode       InstallTypeValueFlag
	TestConfig        TestConfigFlag
	ClusterConfig     ClusterConfigFlag
	SUCUpgradeVersion SUCUpgradeVersion
	Channel           ChannelFlag
}

type SUCUpgradeVersion struct {
	Version string
}

type InstallTypeValueFlag struct {
	Version string
	Commit  string
}

type ChannelFlag struct {
	Channel string
}

type TestConfigFlag struct {
	TestFuncNames  []string
	TestFuncs      []TestCaseFlag
	DeployWorkload bool
	WorkloadName   string
	Description    string
}

type ClusterConfigFlag struct {
	Destroy DestroyFlag
	Arch    ArchFlag
}

type DestroyFlag bool

type ArchFlag string

type TestCaseFlag func(deployWorkload bool)

type StringSlice []string

func (s *StringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *StringSlice) Set(value string) error {
	*s = strings.Split(value, ",")
	return nil
}

func (t *TestConfigFlag) String() string {
	return fmt.Sprintf("TestFuncName: %s", t.TestFuncNames)
}

func (t *TestConfigFlag) Set(value string) error {
	t.TestFuncNames = strings.Split(value, ",")
	return nil
}

func (c *ChannelFlag) String() string {
	return c.Channel
}

func (c *ChannelFlag) Set(value string) error {
	if value == "" {
		return nil
	}

	if value != "latest" && value != "stable" && value != "testing" {
		return shared.ReturnLogError("invalid channel: %s", value)
	}

	c.Channel = value

	return nil
}

func (i *InstallTypeValueFlag) String() string {
	return fmt.Sprintf("%s%s", i.Version, i.Commit)
}

func (i *InstallTypeValueFlag) Set(value string) error {
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

func (t *SUCUpgradeVersion) String() string {
	return t.Version
}

func (t *SUCUpgradeVersion) Set(value string) error {
	if !strings.HasPrefix(value, "v") || !strings.HasSuffix(value, "rke2r1") {
		return shared.ReturnLogError("invalid version format: %s", value)
	}

	t.Version = value
	return nil
}

func (d *DestroyFlag) String() string {
	return fmt.Sprintf("%v", *d)
}

func (d *DestroyFlag) Set(value string) error {
	v, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	*d = DestroyFlag(v)

	return nil
}

func (a *ArchFlag) String() string {
	return string(*a)
}

func (a *ArchFlag) Set(value string) error {
	if value == "arm" || value == "arm64" ||
		value == "amd64" || value == "s390x" {
		*a = ArchFlag(value)
	} else {
		*a = "amd64"
	}

	return nil
}
