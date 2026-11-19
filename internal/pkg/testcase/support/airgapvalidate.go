package support

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// Validation-only helpers for airgap clusters that qainfra already installed.
// Every command runs on a private node through the bastion (CmdForPrivateNode).

// egressProbe connects by IP with TLS verification off: a timeout (rc 28) or rc 7 with
// "unreachable"/"No route" proves no path out; a refused connection means a host answered.
const egressProbe = "for ip in 140.82.112.3 1.1.1.1; do " +
	"msg=$(curl -k -sS --max-time 5 -o /dev/null https://$ip/ 2>&1 >/dev/null); rc=$?; " +
	"echo \"$ip rc=$rc $msg\"; done"

// ValidateAirgapIsolation proves no node reaches the internet while each keeps a
// default route (kube-proxy needs one).
func ValidateAirgapIsolation(cluster *driver.Cluster) error {
	for _, node := range append(append([]string{}, cluster.ServerIPs...), cluster.AgentIPs...) {
		out, err := CmdForPrivateNode(cluster, egressProbe, node)
		if err != nil {
			return fmt.Errorf("isolation probe on %s: %w", node, err)
		}
		if err := assertEgressBlocked(out); err != nil {
			return fmt.Errorf("node %s: %w", node, err)
		}

		route, err := CmdForPrivateNode(cluster, "ip route show default", node)
		if err != nil || strings.TrimSpace(route) == "" {
			return fmt.Errorf("node %s has no default route (kube-proxy needs one): %v %q", node, err, route)
		}

		// A global IPv6 address reaches the internet through a plain IGW; the probes above are IPv4 only.
		v6, err := CmdForPrivateNode(cluster, "ip -6 addr show scope global", node)
		if err != nil {
			return fmt.Errorf("ipv6 check on %s: %w", node, err)
		}
		if strings.TrimSpace(v6) != "" {
			return fmt.Errorf("node %s has a global IPv6 address; the subnet is not airgapped: %q", node, v6)
		}
	}
	resources.LogLevel("info", "Isolation verified on %d nodes: egress blocked, no global IPv6, default route present",
		len(cluster.ServerIPs)+len(cluster.AgentIPs))

	return nil
}

// assertEgressBlocked parses "<ip> rc=<n> <curl message>" lines from egressProbe.
func assertEgressBlocked(out string) error {
	seen := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "rc=") {
			continue
		}
		seen++
		msg := strings.ToLower(strings.Join(fields[2:], " "))
		noRoute := strings.Contains(msg, "unreachable") || strings.Contains(msg, "no route")
		switch rc := strings.TrimPrefix(fields[1], "rc="); {
		case rc == "28", rc == "7" && noRoute:
			// timed out or no route at all: no path to the internet
		case rc == "0", rc == "7", rc == "35", rc == "51", rc == "60":
			return fmt.Errorf("reached %s (curl rc=%s %s); the subnet is not airgapped", fields[0], rc, msg)
		default:
			return fmt.Errorf("egress probe to %s inconclusive (curl rc=%s): %q", fields[0], rc, out)
		}
	}
	if seen == 0 {
		return fmt.Errorf("egress probe produced no result lines: %q", out)
	}

	return nil
}

// ValidateAirgapTarball checks the image tarballs were staged for the runtime import.
func ValidateAirgapTarball(cluster *driver.Cluster) error {
	dir := fmt.Sprintf("/var/lib/rancher/%s/agent/images", cluster.Config.Product)
	for _, ip := range append(append([]string{}, cluster.ServerIPs...), cluster.AgentIPs...) {
		out, err := CmdForPrivateNode(cluster, "sudo ls "+dir, ip)
		if err != nil {
			return fmt.Errorf("list %s on %s: %w", dir, ip, err)
		}
		if !strings.Contains(out, "."+cluster.Airgap.TarballType) {
			return fmt.Errorf("no *.%s image tarball in %s on %s: %q", cluster.Airgap.TarballType, dir, ip, out)
		}
	}
	resources.LogLevel("info", "Tarball images present under %s on every node", dir)

	return nil
}

// ValidateAirgapPrivateRegistry checks registries.yaml on every node, TLS and
// auth on the registry, and that a node can pull a published image through it.
func ValidateAirgapPrivateRegistry(cluster *driver.Cluster) error {
	product := cluster.Config.Product
	regFile := fmt.Sprintf("/etc/rancher/%s/registries.yaml", product)
	host := cluster.Airgap.RegistryHost
	for _, ip := range append(append([]string{}, cluster.ServerIPs...), cluster.AgentIPs...) {
		out, err := CmdForPrivateNode(cluster, "sudo grep -c "+host+" "+regFile, ip)
		if err != nil || strings.TrimSpace(out) == "0" {
			return fmt.Errorf("%s on %s does not reference %s: %v %q", regFile, ip, host, err, out)
		}
	}

	node := cluster.ServerIPs[0]
	// Unauthenticated access must be refused, authenticated access (via the node's CA) accepted.
	probe := fmt.Sprintf("curl -s -o /dev/null -w %%{http_code} --cacert %s https://%s/v2/",
		cluster.Airgap.RegistryCAPathNodes, host)
	code, err := CmdForPrivateNode(cluster, probe, node)
	if err != nil || strings.TrimSpace(code) != "401" {
		return fmt.Errorf("registry %s must require auth (want 401): %v %q", host, err, code)
	}

	return pullThroughRegistry(cluster, node, host)
}

// ValidateAirgapSystemDefaultRegistry checks the product config points at the
// bastion registry, the CA is trusted and system images carry the registry prefix.
func ValidateAirgapSystemDefaultRegistry(cluster *driver.Cluster) error {
	product := cluster.Config.Product
	host := cluster.Airgap.RegistryHost
	cfg := fmt.Sprintf("/etc/rancher/%s/config.yaml", product)
	// K3s agents inherit this server-only setting from the server configuration.
	configNodes := append([]string{}, cluster.ServerIPs...)
	if product != "k3s" {
		configNodes = append(configNodes, cluster.AgentIPs...)
	}
	for _, ip := range configNodes {
		out, err := CmdForPrivateNode(cluster, "sudo grep system-default-registry "+cfg, ip)
		if err != nil || !strings.Contains(out, host) {
			return fmt.Errorf("%s on %s lacks system-default-registry: %s: %v %q", cfg, ip, host, err, out)
		}
	}
	if product == "k3s" {
		for _, ip := range cluster.AgentIPs {
			if err := validateK3sAgentRegistry(cluster, ip, host); err != nil {
				return err
			}
		}
	}

	node := cluster.ServerIPs[0]
	code, err := CmdForPrivateNode(cluster,
		fmt.Sprintf("curl -s --max-time 20 -o /dev/null -w %%{http_code} https://%s/v2/", host), node)
	if err != nil || strings.TrimSpace(code) != "401" {
		return fmt.Errorf("registry %s must require auth with the OS trust store (want 401; needs a QA-infra "+
			"airgap revision that authenticates this registry): %v %q", host, err, code)
	}

	if imageErr := requireRegistryImages(cluster, product, node, host); imageErr != nil {
		return imageErr
	}

	return pullThroughRegistry(cluster, node, host)
}

// Check the generated sandbox image before any workload needs to pull it.
func validateK3sAgentRegistry(cluster *driver.Cluster, node, host string) error {
	cmd := "sudo grep -E '^[[:space:]]*(sandbox_image|sandbox)[[:space:]]*=' " +
		"/var/lib/rancher/k3s/agent/etc/containerd/config.toml"
	config, configErr := CmdForPrivateNode(cluster, cmd, node)
	if configErr != nil {
		return fmt.Errorf("read containerd sandbox image on %s: %w", node, configErr)
	}

	if registryErr := assertK3sSandboxRegistry(config, host); registryErr != nil {
		return fmt.Errorf("containerd sandbox image on %s: %w", node, registryErr)
	}

	return nil
}

var sandboxImageRE = regexp.MustCompile(`(?m)^\s*(?:sandbox_image|sandbox)\s*=\s*["']([^"'\r\n]+)["'][ \t]*(?:#.*)?$`)

func assertK3sSandboxRegistry(config, host string) error {
	images := sandboxImageRE.FindAllStringSubmatch(config, -1)
	if len(images) != 1 || !strings.HasPrefix(images[0][1], host+"/") {
		return fmt.Errorf("expected one sandbox image with the %s/ prefix", host)
	}

	return nil
}

// requireRegistryImages fails unless some image on the node carries the registry host prefix.
func requireRegistryImages(cluster *driver.Cluster, product, node, host string) error {
	images, err := CmdForPrivateNode(cluster, crictlImagesCmd(product), node)
	if err != nil {
		return fmt.Errorf("crictl images on %s: %w", node, err)
	}
	if !strings.Contains(images, host+"/") {
		return fmt.Errorf("no image on %s carries the %s/ prefix; system-default-registry was not applied", node, host)
	}

	return nil
}

// pullThroughRegistry pulls one published image explicitly from the bastion
// registry and proves an unpublished reference fails.
func pullThroughRegistry(cluster *driver.Cluster, node, host string) error {
	product := cluster.Config.Product
	listFile := "k3s-images.txt"
	if product == "rke2" {
		listFile = "rke2-images*.txt"
	}
	first, err := resources.RunCommandOnNode(
		fmt.Sprintf("cat %s/%s | head -1", cluster.Airgap.ArtifactsDir, listFile), cluster.Bastion.PublicIPv4Addr)
	if err != nil || strings.TrimSpace(first) == "" {
		return fmt.Errorf("read image list on bastion: %w %q", err, first)
	}
	image := strings.TrimSpace(first)
	if i := strings.Index(image, "/"); i > 0 && (strings.ContainsAny(image[:i], ".:") || image[:i] == "localhost") {
		image = image[i+1:]
	}

	if out, err := CmdForPrivateNode(cluster, crictlPullCmd(product, host+"/"+image), node); err != nil {
		return fmt.Errorf("pull %s/%s through the bastion registry failed: %w %q", host, image, err, out)
	}
	if out, err := CmdForPrivateNode(cluster, crictlPullCmd(product, host+"/dtf/not-published:1"), node); err == nil {
		return fmt.Errorf("pull of an unpublished image must fail, got: %q", out)
	}
	resources.LogLevel("info", "Registry %s serves %s and refuses unpublished references", host, image)

	return nil
}

// k3sBinary is absolute because SLES/RHEL sudo secure_path omits /usr/local/bin.
const k3sBinary = "/usr/local/bin/k3s"

func crictlImagesCmd(product string) string {
	if product == "k3s" {
		return "sudo " + k3sBinary + " crictl images"
	}

	return "sudo /var/lib/rancher/rke2/bin/crictl --config /var/lib/rancher/rke2/agent/etc/crictl.yaml images"
}

func crictlPullCmd(product, image string) string {
	if product == "k3s" {
		return "sudo " + k3sBinary + " crictl pull " + image
	}

	return "sudo /var/lib/rancher/rke2/bin/crictl --config /var/lib/rancher/rke2/agent/etc/crictl.yaml pull " + image
}
