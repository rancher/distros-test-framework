package resources

import "fmt"

// InstallHelm installs helm on the container.
func InstallHelm() (res string, err error) {
	// Install Helm from local tarball
	cmd := fmt.Sprintf("tar -zxvf %v/bin/helm-v3.18.3-linux-amd64.tar.gz -C /tmp && "+
		"cp /tmp/linux-amd64/helm /usr/local/bin/helm && "+
		"chmod +x /usr/local/bin/helm && "+
		"helm version", BasePath())

	return RunCommandHost(cmd)
}

// CheckHelmRepo checks a helm chart is available on the repo.
func CheckHelmRepo(name, url, version string) (string, error) {
	addRepo := fmt.Sprintf("helm repo add %s %s", name, url)
	update := "helm repo update"
	searchRepo := fmt.Sprintf("helm search repo %s --devel -l | grep %s", name, version)

	return RunCommandHost(addRepo, update, searchRepo)
}
