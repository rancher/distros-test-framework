package support

import (
	"fmt"
	"strings"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// Validation-only helpers for airgap clusters that qainfra already installed.
// Every command runs on a private node through the bastion (CmdForPrivateNode).

// egressProbe connects by IP with TLS verification off: only a timeout (curl rc 28)
// proves no path out; "refused" (rc 7) means a host answered, so it is not a pass.
const egressProbe = "for ip in 140.82.112.3 1.1.1.1; do " +
	"curl -k -sS --max-time 5 -o /dev/null https://$ip/ >/dev/null 2>&1; echo \"$ip rc=$?\"; done"

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

// assertEgressBlocked parses "<ip> rc=<n>" lines from egressProbe.
func assertEgressBlocked(out string) error {
	seen := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !strings.HasPrefix(fields[1], "rc=") {
			continue
		}
		seen++
		switch rc := strings.TrimPrefix(fields[1], "rc="); rc {
		case "28":
			// timed out: no path to the internet
		case "0", "7", "35", "51", "60":
			return fmt.Errorf("reached %s (curl rc=%s); the subnet is not airgapped", fields[0], rc)
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
	for _, ip := range append(append([]string{}, cluster.ServerIPs...), cluster.AgentIPs...) {
		out, err := CmdForPrivateNode(cluster, "sudo grep system-default-registry "+cfg, ip)
		if err != nil || !strings.Contains(out, host) {
			return fmt.Errorf("%s on %s lacks system-default-registry: %s: %v %q", cfg, ip, host, err, out)
		}
	}

	node := cluster.ServerIPs[0]
	code, err := CmdForPrivateNode(cluster,
		fmt.Sprintf("curl -s -o /dev/null -w %%{http_code} https://%s/v2/", host), node)
	if err != nil || strings.TrimSpace(code) != "200" {
		return fmt.Errorf("registry %s must be reachable with the OS trust store (want 200): %v %q", host, err, code)
	}

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
	if i := strings.Index(image, "/"); i > 0 && (strings.Contains(image[:i], ".") || image[:i] == "docker.io") {
		image = image[i+1:]
	}

	if out, err := CmdForPrivateNode(cluster, crictlPullCmd(product, host+"/"+image), node); err != nil {
		return fmt.Errorf("pull %s/%s through the private registry failed: %w %q", host, image, err, out)
	}
	if out, err := CmdForPrivateNode(cluster, crictlPullCmd(product, host+"/dtf/not-published:1"), node); err == nil {
		return fmt.Errorf("pull of an unpublished image must fail, got: %q", out)
	}
	resources.LogLevel("info", "Registry %s serves %s and refuses unpublished references", host, image)

	return nil
}

func crictlImagesCmd(product string) string {
	if product == "k3s" {
		return "sudo k3s crictl images"
	}

	return "sudo /var/lib/rancher/rke2/bin/crictl --config /var/lib/rancher/rke2/agent/etc/crictl.yaml images"
}

func crictlPullCmd(product, image string) string {
	if product == "k3s" {
		return "sudo k3s crictl pull " + image
	}

	return "sudo /var/lib/rancher/rke2/bin/crictl --config /var/lib/rancher/rke2/agent/etc/crictl.yaml pull " + image
}
