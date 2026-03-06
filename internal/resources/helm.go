package resources

import (
	"fmt"
	"strings"
)

// InstallHelm installs helm on the container.
func InstallHelm() (res string, err error) {
	// Install Helm from local tarball
	cmd := fmt.Sprintf("tar -zxvf %v/bin/helm-v3.18.3-linux-amd64.tar.gz -C /tmp && "+
		"cp /tmp/linux-amd64/helm /usr/local/bin/helm && "+
		"chmod +x /usr/local/bin/helm && "+
		"helm version", BasePath())

	return RunCommandHost(cmd)
}

func isHelmInstalled(ip string, helmVersion string) bool {
	cmd := "helm version"
	res, err := RunCommandOnNode(cmd, ip)
	if err != nil {
		return false
	}
	return strings.Contains(res, helmVersion)
}

func InstallHelmOnNode(ip string) (res string, err error) {
	// Install Helm from local tarball
	helmVersion, err := GetLatestReleaseTag("helm", "helm")
	if err != nil {
		return helmVersion, err
	}

	LogLevel("info", "Latest helm version: %s\n", helmVersion)
	if !isHelmInstalled(ip, helmVersion) {
		cmd := fmt.Sprintf(`wget -P /tmp https://get.helm.sh/helm-%s-linux-amd64.tar.gz && \
	tar -zxvf /tmp/helm-%s-linux-amd64.tar.gz -C /tmp && \
	sudo cp /tmp/linux-amd64/helm /usr/local/bin/helm && \
	sudo chmod +x /usr/local/bin/helm && \
	helm version`, helmVersion, helmVersion)
		res, err = RunCommandOnNode(cmd, ip)
		if err != nil {
			return res, err
		}
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
