package customflag

import (
	"errors"
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
	InstallMode          installModeFlag
	TestTemplateConfig   templateConfigFlag
	Destroy              destroyFlag
	SUCUpgradeVersion    sucUpgradeVersionFlag
	Channel              channelFlag
	External             externalFlag
	CertManager          certManagerFlag
	Charts               helmChartsFlag
	AirgapFlag           airgapFlag
	S3Flags              s3ConfigFlag
	SelinuxTest          selinuxTestFlag
	KillAllUninstallTest killalluninstallTestFlag
	SecretsEncrypt       secretsEncryptFlag
}

// TestMapConfig is a type that wraps the test commands and expected values.
type TestMapConfig testMapConfigFlag

// testMapConfigFlag represents a single test command with key:value pairs.
type testMapConfigFlag struct {
	Cmd                        string
	ExpectedValue              string
	ExpectedValueUpgrade       string
	ExpectedChartsValue        string
	ExpectedChartsValueUpgrade string
}

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

type airgapFlag struct {
	ImageRegistryUrl string
	RegistryUsername string
	RegistryPassword string
	TarballType      string
}

type TestCaseFlag func(applyWorkload, deleteWorkload bool)

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

type externalFlag struct {
	SonobuoyVersion string
}

func (e *externalFlag) String() string {
	return e.SonobuoyVersion
}

func (e *externalFlag) Set(value string) error {
	e.SonobuoyVersion = value

	return nil
}

type helmChartsFlag struct {
	Args     string
	Version  string
	RepoName string
	RepoUrl  string
}

type certManagerFlag struct {
	Version string
}

type s3ConfigFlag struct {
	Bucket string
	Folder string
}

type selinuxTestFlag bool

func (s *selinuxTestFlag) Set(value string) error {
	if value == "" {
		return errors.New("invalid selinux test flag - cannot be empty")
	}

	// selinux test flag can only be true or false
	// if value is not true or false, return an error
	if value != "true" && value != "false" {
		return fmt.Errorf("invalid selinux test flag: %s", value)
	}

	v, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}

	*s = selinuxTestFlag(v)

	return nil
}

func (s *selinuxTestFlag) String() string {
	return fmt.Sprintf("%v", *s)
}

type secretsEncryptFlag struct {
	Method string
}

type killalluninstallTestFlag bool

func (d *killalluninstallTestFlag) String() string {
	return fmt.Sprintf("%v", *d)
}

func (d *killalluninstallTestFlag) Set(value string) error {
	v, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	*d = killalluninstallTestFlag(v)

	return nil
}
