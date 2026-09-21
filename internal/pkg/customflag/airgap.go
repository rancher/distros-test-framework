package customflag

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// The airgap scenario is the suite build tag, seen as TEST_TAG (local) or as
// -tags= inside TEST_ARGS (Jenkins); suite and qainfra provisioner both use these.
const (
	TestTagEnv  = "TEST_TAG"
	TestArgsEnv = "TEST_ARGS"
)

var airgapMethods = map[string]string{
	"tarball":               "tarball",
	"privateregistry":       "private_registry",
	"systemdefaultregistry": "system_default_registry",
}

var airgapVersionRE = map[string]*regexp.Regexp{
	"k3s":  regexp.MustCompile(`^v\d+\.\d+\.\d+(-[a-z0-9.]+)?\+k3s\d+$`),
	"rke2": regexp.MustCompile(`^v\d+\.\d+\.\d+(-[a-z0-9.]+)?\+rke2r\d+$`),
}

// TestTag returns the suite build tag: TEST_TAG, else the first -tags= value in TEST_ARGS.
func TestTag() string {
	if tag := strings.TrimSpace(os.Getenv(TestTagEnv)); tag != "" {
		return tag
	}
	args := os.Getenv(TestArgsEnv)
	i := strings.Index(args, "-tags=")
	if i == -1 {
		return ""
	}
	tag := strings.Fields(args[i+len("-tags="):] + " ")[0]

	return strings.Split(tag, ",")[0]
}

// AirgapMethod maps the suite tag onto the playbook scenario.
func AirgapMethod() (string, error) {
	tag := TestTag()
	method, ok := airgapMethods[tag]
	if !ok {
		return "", fmt.Errorf("airgap needs %s or -tags= in %s set to tarball|privateregistry|systemdefaultregistry, got %q",
			TestTagEnv, TestArgsEnv, tag)
	}

	return method, nil
}

// ValidateAirgapInputs rejects an unusable scenario (method, tarball type,
// version, Prime URL) before anything is provisioned.
func ValidateAirgapInputs(product, installVersion string, af *airgapFlag) error {
	if _, err := AirgapMethod(); err != nil {
		return err
	}
	switch strings.TrimSpace(af.TarballType) {
	case "", "tar.zst", "tar.gz":
	default:
		return fmt.Errorf("tarballType %q must be tar.zst or tar.gz", af.TarballType)
	}
	re, ok := airgapVersionRE[strings.ToLower(strings.TrimSpace(product))]
	if !ok {
		return fmt.Errorf("airgap supports k3s|rke2, got product %q", product)
	}
	if !re.MatchString(strings.TrimSpace(installVersion)) {
		return fmt.Errorf("airgap needs a release version for %s (e.g. v1.36.0+%s), got %q; "+
			"commit installs are not supported offline",
			product, map[string]string{"k3s": "k3s1", "rke2": "rke2r1"}[product], installVersion)
	}
	url := strings.TrimSpace(af.ImageRegistryUrl)
	if url != "" && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return fmt.Errorf("imageRegistryUrl %q must be an http(s) URL", url)
	}

	return nil
}
