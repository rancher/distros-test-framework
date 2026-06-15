package resources

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
)

// InstallHelm installs helm on the container. Uses /usr/local/bin when the
// process runs as root and that path is writable; otherwise falls back to
// ~/bin. That fallback keeps the helper working on immutable OS images where
// /usr/local is read-only (Elemental3 / UnifiedCore). Selects the right
// tarball based on the "arch" env var (falls back to runtime.GOARCH).
func InstallHelm() (res string, err error) {
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
	localbin, err := getBinPath()
	if err != nil {
		return "", fmt.Errorf("unable to get binary path for helm install: %w", err)
	}
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

// getBinPath picks the binary install directory. Prefers /usr/local/bin when
// the process runs as root and the path is writable; falls back to ~/bin
// otherwise (non-root, Windows, or read-only /usr/local on immutable OS
// images like Elemental3 / UnifiedCore).
func getBinPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", ReturnLogError("unable to get user home dir: %w", err)
	}
	binPath := filepath.Join(homeDir, "bin")

	// os.Getuid() returns 0 for root on Linux/macOS, -1 on Windows.
	if os.Getuid() == 0 {
		rootPath := "/usr/local/bin"
		_ = os.MkdirAll(rootPath, 0o755)

		// Write test — detects read-only filesystems (Elemental3 / UnifiedCore).
		f, wErr := os.CreateTemp(rootPath, ".write_test_*")
		if wErr == nil {
			name := f.Name()
			if cErr := f.Close(); cErr != nil {
				return "", ReturnLogError("unable to close write-test file: %w", cErr)
			}
			_ = os.Remove(name)
			binPath = rootPath
		}
	}

	if err := os.MkdirAll(binPath, 0o755); err != nil {
		return "", ReturnLogError("unable to mkdir %q: %w", binPath, err)
	}

	return binPath, nil
}
