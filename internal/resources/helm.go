package resources

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
)

// InstallHelm installs helm on the container. Installs into ~/bin so the
// helper works on immutable OS images where /usr/local is read-only
// (Elemental3 / UnifiedCore). Selects the right tarball based on the "arch"
// env var (falls back to runtime.GOARCH).
func InstallHelm() (res string, err error) {
	// get home directory
	homedir, err := os.UserHomeDir()
	if err != nil {
		return "", ReturnLogError("failed to get home dir: %w", err)
	}

	// get targeted architecture
	arch := os.Getenv("arch")
	if arch == "" {
		arch = runtime.GOARCH
	}

	switch arch {
	case "x86_64":
		arch = "amd64"
	case "amd64", "arm64":
		// Supported as-is
	default:
		return "", ReturnLogError("unsupported architecture for Helm installation: %q", arch)
	}

	// install Helm from local tarball
	localbin := fmt.Sprintf("%v/bin", homedir)
	cmd := fmt.Sprintf("mkdir -p %v && "+
		"tar -zxvf %v/bin/helm-v*-linux-%v*.tar.gz -C /tmp && "+
		"cp /tmp/linux-%v*/helm %v/helm && "+
		"chmod +x %v/helm && "+
		"%v/helm version", localbin, BasePath(), arch, arch, localbin, localbin, localbin)

	res, err = RunCommandHost(cmd)
	if err != nil {
		return res, err
	}

	// Make the freshly-installed binary discoverable by subsequent bare
	// `helm` calls (CheckHelmRepo, deployrancher's `helm install`/`helm repo
	// add`, etc.). RunCommandHost exec's bash subprocesses that inherit the
	// Go process env, so prepending to $PATH here is enough.
	if path := os.Getenv("PATH"); !slices.Contains(filepath.SplitList(path), localbin) {
		//nolint:revive //no need to check the error.
		os.Setenv("PATH", localbin+string(os.PathListSeparator)+path)
	}

	return res, nil
}

// CheckHelmRepo checks a helm chart is available on the repo.
func CheckHelmRepo(name, url, version string) (string, error) {
	addRepo := fmt.Sprintf("helm repo add %s %s", name, url)
	update := "helm repo update"
	searchRepo := fmt.Sprintf("helm search repo %s --devel -l | grep %s", name, version)

	return RunCommandHost(addRepo, update, searchRepo)
}
