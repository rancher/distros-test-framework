package cmd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var ServiceFlag FlagConfig
var TestCaseNameFlag StringSlice

type FlagConfig struct {
	InstallType       InstallTypeValueFlag
	InstallUpgrade    MultiValueFlag
	TestConfig        TestConfigFlag
	ClusterConfig     ClusterConfigFlag
	KubeConfigFile	  KubeConfigFileFlag
	UpgradeVersionSUC UpgradeVersionFlag
}

// UpgradeVersionFlag is a custom type to use upgradeVersionSUC flag
type UpgradeVersionFlag struct {
	Version string
}

// InstallTypeValueFlag is a cmd type that can be used to parse the installation type
type InstallTypeValueFlag struct {
	Version []string
	Commit  []string
	Channel string
}

// TestConfigFlag is a cmd type that can be used to parse the test case argument
type TestConfigFlag struct {
	TestFuncNames  []string
	TestFuncs      []TestCaseFlag
	DeployWorkload bool
	WorkloadName   string
	Description    string
}

// TestCaseFlag is a cmd type that can be used to parse the test case argument
type TestCaseFlag func(deployWorkload bool)

// MultiValueFlag is a cmd type that can be used to parse multiple values
type MultiValueFlag []string

// DestroyFlag is a cmd type that can be used to parse the destroy flag
type DestroyFlag bool

// ArchFlag is a cmd type that can be used to parse the destroy flag
type ArchFlag string

// KubeConfigFileFlag is a cli flag type that can be used to run tests on existing cluster
type KubeConfigFileFlag string

// Product is a cli flag type that can be used to select product for creating cluster
type ProductFlag string

// ClusterConfigFlag is a cmd type that can be used to change some cluster config
type ClusterConfigFlag struct {
	KubeConfigFile	KubeConfigFileFlag		
	Destroy 		DestroyFlag
	Arch    		ArchFlag
	Product			ProductFlag
}

// StringSlice defines a custom flag type for string slice
type StringSlice []string

// String returns the string representation of the StringSlice
func (s *StringSlice) String() string {
	return strings.Join(*s, ",")
}

// Set parses the input string and sets the StringSlice using Set cmd interface
func (s *StringSlice) Set(value string) error {
	*s = strings.Split(value, ",")
	return nil
}

// String returns the string representation of the MultiValueFlag
func (m *MultiValueFlag) String() string {
	return strings.Join(*m, ",")
}

// Set func sets multiValueFlag appending the value
func (m *MultiValueFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

// String returns the string representation of the TestConfigFlag
func (t *TestConfigFlag) String() string {
	return fmt.Sprintf("TestFuncName: %s", t.TestFuncNames)
}

// Set parses the cmd value for TestConfigFlag
func (t *TestConfigFlag) Set(value string) error {
	t.TestFuncNames = strings.Split(value, ",")
	return nil
}

// String returns the string representation of the InstallTypeValue
func (i *InstallTypeValueFlag) String() string {
	return fmt.Sprintf("Version: %s, Commit: %s", i.Version, i.Commit)
}

// Set parses the input string and sets the Version or Commit field using Set cmd interface
func (i *InstallTypeValueFlag) Set(value string) error {
	parts := strings.Split(value, "=")

	for _, part := range parts {
		subParts := strings.Split(part, "=")
		if len(subParts) != 2 {
			return fmt.Errorf("invalid input format")
		}
		switch parts[0] {
		case "INSTALL_RKE2_VERSION", "INSTALL_K3S_VERSION":
			i.Version = append(i.Version, subParts[1])
		case "INSTALL_RKE2_COMMIT", "INSTALL_K3S_COMMIT":
			i.Commit = append(i.Commit, subParts[1])
		default:
			return fmt.Errorf("invalid install type: %s", parts[0])
		}
	}

	return nil
}

// String returns the string representation of the UpgradeVersion for SUC upgrade
func (t *UpgradeVersionFlag) String() string {
	return t.Version
}

// Set parses the input string and sets the Version field for SUC upgrades
func (t *UpgradeVersionFlag) Set(value string) error {
	regMatch, err := regexp.MatchString("(rke2r|k3s[1-9])", value)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(value, "v") && !regMatch {
		return fmt.Errorf("invalid install format: %s", value)
	}
	t.Version = value
	return nil
}

// String returns the string representation of the ArchFlag
func (a *ArchFlag) String() string {
	return string(*a)
}

// String returns the string representation of the ArchFlag
func (a *KubeConfigFileFlag) String() string {
	return string(*a)
}

// Set parses the file for KubeConfigFileFlag
func (a *KubeConfigFileFlag) Set(value string) error {
	*a = KubeConfigFileFlag(value)
	fmt.Println(value)
	return nil
}

// String returns the string representation of the ArchFlag
func (a *ProductFlag) String() string {
	return string(*a)
}

// Set parses the file for KubeConfigFileFlag
func (a *ProductFlag) Set(value string) error {
	*a = ProductFlag(value)
	fmt.Println("Using product for creating cluster: ",value)
	return nil
}

// Set parses the cmd value for ArchFlag
func (a *ArchFlag) Set(value string) error {
	if value == "arm64" || value == "amd64" {
		*a = ArchFlag(value)
	} else {
		*a = "amd64"
	}

	return nil
}

// String returns the string representation of the DestroyFlag
func (d *DestroyFlag) String() string {
	return fmt.Sprintf("%v", *d)
}

// Set parses the cmd value for DestroyFlag
func (d *DestroyFlag) Set(value string) error {
	v, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	*d = DestroyFlag(v)

	return nil
}
