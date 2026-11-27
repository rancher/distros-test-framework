package testcase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/pkg/aws"
	"github.com/rancher/distros-test-framework/internal/pkg/rancher"
	"github.com/rancher/distros-test-framework/internal/pkg/ui"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	day1CustomCluster = "day1-custom"
	day1ImportCluster = "day1-import"
	day1Marker        = "dtf-day1-marker"
	provisionSelect   = `[data-testid="clusterBasics__kubernetesVersions"]`
	importedSelect    = `[data-testid="cruimported-kubernetesversion"]`
	clusterListPath   = "/dashboard/c/_/manager/provisioning.cattle.io.cluster"
)

// RancherDay1Config is the validated input of the day-1 flow.
type RancherDay1Config struct {
	Product         string
	Version         string // under test: custom cluster version and imported upgrade target
	PreviousVersion string // released version the imported cluster starts on
	DataURL         string
	ResourceName    string
	CustomNodes     [][]string // roles per custom cluster node, from rancher.ParseLayout
	ImportNodes     [][]string // roles per imported cluster node; servers first
}

// RancherDay1 carries state between the ordered Rancher day-1 specs.
type RancherDay1 struct {
	RancherDay1Config
	cluster      *driver.Cluster
	artifactsDir string
	baseURL      string

	api       *rancher.Client
	browser   *ui.Browser
	awsClient *aws.Client

	customID, customKubectlIP string
	importID, importKubectlIP string
	nodeIPs                   map[string]string // node name -> public IP, for debug collection
	customReady               bool
	snapshot, snapshotFile    string
	debugRuns                 int
}

// NewRancherDay1 prepares the day-1 flow; versions must already be validated against data.json.
func NewRancherDay1(cluster *driver.Cluster, cfg *RancherDay1Config) *RancherDay1 {
	return &RancherDay1{
		RancherDay1Config: *cfg,
		cluster:           cluster,
		artifactsDir:      filepath.Join(resources.BasePath(), "artifacts", "rancherday1"),
		baseURL:           "https://" + cluster.FQDN,
		nodeIPs:           map[string]string{},
	}
}

// ConnectRancher completes first-login setup, logs in through the API and opens an authenticated browser.
func (r *RancherDay1) ConnectRancher() {
	encoded, err := rancherKubectl("get", "secret", "-n", "cattle-system", "bootstrap-secret",
		"-o", "jsonpath={.data.bootstrapPassword}")
	Expect(err).NotTo(HaveOccurred(), "read rancher bootstrap-secret")
	password, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	Expect(err).NotTo(HaveOccurred(), "decode rancher bootstrap password")

	// The dashboard sends a fresh admin to the setup wizard unless first login is already complete.
	setRancherSetting("first-login", "false")
	setRancherSetting("eula-agreed", time.Now().UTC().Format(time.RFC3339))
	serverURL, _ := rancherKubectl("get", "settings.management.cattle.io", "server-url", "-o", "jsonpath={.value}")
	if serverURL == "" {
		setRancherSetting("server-url", r.baseURL)
	}
	adminUser, err := rancherKubectl("get", "users.management.cattle.io",
		"-o", `jsonpath={range .items[?(@.username=="admin")]}{.metadata.name}{end}`)
	Expect(err).NotTo(HaveOccurred(), "find rancher admin user")
	Expect(adminUser).NotTo(BeEmpty(), "rancher admin user not found")
	_, err = rancherKubectl("patch", "users.management.cattle.io", adminUser, "--type=merge",
		"-p", `{"mustChangePassword":false}`)
	Expect(err).NotTo(HaveOccurred(), "clear admin mustChangePassword")

	caPEM, err := rancherKubectl("get", "settings.management.cattle.io", "cacerts", "-o", "jsonpath={.value}")
	Expect(err).NotTo(HaveOccurred(), "read rancher cacerts setting")
	r.api, err = rancher.NewClient(r.baseURL, caPEM)
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() error {
		return r.api.Login(context.Background(), "admin", string(password))
	}, "5m", "15s").Should(Succeed(), "log in to rancher API at %s", r.baseURL)

	spki := ""
	if caPEM != "" {
		spki, err = rancher.LeafSPKI(context.Background(), r.baseURL, caPEM)
		Expect(err).NotTo(HaveOccurred())
	}
	r.browser, err = ui.New(ui.Options{
		BaseURL:      r.baseURL,
		SessionToken: r.api.Token(),
		TrustedSPKI:  spki,
		ArtifactsDir: filepath.Join(r.artifactsDir, "screenshots"),
	})
	Expect(err).NotTo(HaveOccurred(), "start headless browser")
	r.browser.AddSecrets(string(password))

	r.openAndShoot(clusterListPath, "local", "rancher-logged-in")
}

// LoadKDM points Rancher at the data.json under test and waits until its channel server offers the version.
func (r *RancherDay1) LoadKDM() {
	r.requireAPI()

	raw, err := rancherKubectl("get", "settings.management.cattle.io", "rke-metadata-config", "-o", "jsonpath={.value}")
	Expect(err).NotTo(HaveOccurred(), "read rke-metadata-config")
	cfg := map[string]string{"refresh-interval-minutes": "1440"}
	if strings.TrimSpace(raw) != "" {
		Expect(json.Unmarshal([]byte(raw), &cfg)).To(Succeed(), "decode rke-metadata-config %q", raw)
	}
	resources.LogLevel("info", "Replacing KDM url %q with %q", cfg["url"], r.DataURL)
	cfg["url"] = r.DataURL
	value, err := json.Marshal(cfg)
	Expect(err).NotTo(HaveOccurred())
	// Rancher refreshes its channel server as soon as this setting changes.
	setRancherSetting("rke-metadata-config", string(value))

	var offered []string
	Eventually(func(g Gomega) {
		offered, err = r.api.ChannelReleases(context.Background(), r.Product)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(offered).To(ContainElement(r.Version))
	}, "10m", "15s").Should(Succeed(),
		"rancher never offered %s from %s; check minChannelServerVersion/maxChannelServerVersion",
		r.Version, r.DataURL)
	resources.LogLevel("info", "Rancher offers %s versions: %v", r.Product, offered)
}

// CheckCreateDropdown screenshots the create-cluster version dropdown and checks the version under test is listed.
func (r *RancherDay1) CheckCreateDropdown() {
	r.requireAPI()
	r.checkDropdown(clusterListPath+"/create?type=custom#basic", provisionSelect, "create", r.Version)
}

// ProvisionCustom creates a custom cluster on the version under test and registers the CustomNodes layout.
func (r *RancherDay1) ProvisionCustom() {
	r.requireAPI()
	id := createRancherCluster(day1CustomCluster, r.Version)
	token := r.registrationToken(id)

	names := make([]string, len(r.CustomNodes))
	for i, roles := range r.CustomNodes {
		names[i] = fmt.Sprintf("day1-custom-%d-%s", i+1, strings.Join(roles, "-"))
	}
	ips, _ := r.createNodes(names...)
	for i, roles := range r.CustomNodes {
		_, err := resources.RunCommandOnNodeWithStdin("sudo sh -s", ips[i],
			strings.NewReader(token.InsecureNodeCommand+" --"+strings.Join(roles, " --")+"\n"))
		Expect(err).NotTo(HaveOccurred(), "register %v node %s to %s", roles, ips[i], day1CustomCluster)
		// etcd-only and worker-only nodes run no apiserver, so check the cluster from a control-plane node.
		if r.customKubectlIP == "" && rancher.CountRole([][]string{roles}, "controlplane") == 1 {
			r.customKubectlIP = ips[i]
		}
	}

	r.waitClusterVersion(day1CustomCluster, id, r.customKubectlIP, r.Version, len(r.CustomNodes), "40m")
	r.customID, r.customReady = id, true
	r.openAndShoot(clusterListPath, day1CustomCluster, "custom-provisioned-"+r.Version)
}

// SnapshotCustom writes a marker ConfigMap and takes an on-demand etcd snapshot that contains it.
func (r *RancherDay1) SnapshotCustom() {
	r.requireCustom()
	kubectl := downstreamKubectl(r.Product)
	_, err := onNode(r.customKubectlIP, kubectl+" delete configmap "+day1Marker+" -n default --ignore-not-found && "+
		kubectl+" create configmap "+day1Marker+" -n default --from-literal=phase=before-snapshot", time.Minute)
	Expect(err).NotTo(HaveOccurred(), "create marker configmap")

	before, err := rancher.SnapshotNames(listSnapshots())
	Expect(err).NotTo(HaveOccurred())
	gen := bumpGeneration("etcdSnapshotCreate", `{"generation":%d}`)
	waitControlPlane("etcd snapshot create", "20m", func(cp *rancher.ControlPlane) bool {
		return cp.SnapshotCreated(gen)
	})

	Eventually(func() error {
		r.snapshot, r.snapshotFile, err = rancher.NewSnapshot(listSnapshots(), before)
		return err
	}, "5m", "15s").Should(Succeed(), "snapshot object for generation %d", gen)
	resources.LogLevel("info", "Created etcd snapshot %s (%s)", r.snapshot, r.snapshotFile)

	r.openAndShoot(clusterListPath+"/"+rancher.FleetNamespace+"/"+day1CustomCluster+"#snapshots",
		r.snapshotFile, "custom-snapshot")
}

// RestoreCustom deletes the marker, restores the snapshot and checks the marker is back.
func (r *RancherDay1) RestoreCustom() {
	r.requireCustom()
	if r.snapshot == "" {
		Skip("no etcd snapshot was taken")
	}
	kubectl := downstreamKubectl(r.Product)
	_, err := onNode(r.customKubectlIP, kubectl+" delete configmap "+day1Marker+" -n default", time.Minute)
	Expect(err).NotTo(HaveOccurred(), "delete marker configmap before restore")

	restore := fmt.Sprintf(`{"name":%q,"generation":%%d,"restoreRKEConfig":"none"}`, r.snapshot)
	gen := bumpGeneration("etcdSnapshotRestore", restore)
	waitControlPlane("etcd snapshot restore", "40m", func(cp *rancher.ControlPlane) bool {
		return cp.SnapshotRestored(gen)
	})

	Eventually(func() error {
		_, err = onNode(r.customKubectlIP, kubectl+" get configmap "+day1Marker+" -n default", time.Minute)
		return err
	}, "10m", "15s").Should(Succeed(), "marker configmap missing after restoring %s", r.snapshot)
	r.waitClusterVersion(day1CustomCluster, r.customID, r.customKubectlIP, r.Version, len(r.CustomNodes), "20m")
	r.openAndShoot(clusterListPath, day1CustomCluster, "custom-restored")
}

// ImportStandalone installs the previous release on the ImportNodes layout and imports it into Rancher.
func (r *RancherDay1) ImportStandalone() {
	r.requireAPI()
	names := make([]string, len(r.ImportNodes))
	for i, roles := range r.ImportNodes {
		kind := "agent"
		if rancher.IsServer(roles) {
			kind = "server"
		}
		names[i] = fmt.Sprintf("day1-import-%d-%s", i+1, kind)
	}
	ips, privateIPs := r.createNodes(names...)
	r.importKubectlIP = ips[0]
	r.installStandalone(ips, privateIPs[0])

	id := createRancherCluster(day1ImportCluster, "")
	token := r.registrationToken(id)
	cmd, err := rancher.ImportCommand(token.InsecureCommand, downstreamKubectl(r.Product))
	Expect(err).NotTo(HaveOccurred())
	Eventually(func() error {
		_, err = resources.RunCommandOnNodeWithStdin("sudo sh -s", r.importKubectlIP, strings.NewReader(cmd+"\n"))
		return err
	}, "10m", "20s").Should(Succeed(), "apply rancher import manifest on %s", r.importKubectlIP)

	r.waitClusterVersion(day1ImportCluster, id, r.importKubectlIP, r.PreviousVersion, len(r.ImportNodes), "20m")
	r.importID = id
	r.openAndShoot(clusterListPath, day1ImportCluster, "imported-"+r.PreviousVersion)
}

// installStandalone starts the first server with cluster-init, then joins the other servers one at a time and the agents.
func (r *RancherDay1) installStandalone(ips []string, firstPrivateIP string) {
	if r.Product == "k3s" {
		// rke2 always runs embedded etcd; k3s needs cluster-init for servers to join.
		r.writeProductConfig(ips[0], "cluster-init: true\n")
	}
	_, err := onNode(ips[0], standaloneInstallCmd(r.Product, r.PreviousVersion, "server"), 15*time.Minute)
	Expect(err).NotTo(HaveOccurred(), "install %s %s server on %s", r.Product, r.PreviousVersion, ips[0])

	var joinToken string
	Eventually(func() (err error) {
		joinToken, err = onNode(ips[0], "sudo cat /var/lib/rancher/"+r.Product+"/server/node-token", time.Minute)
		return err
	}, "5m", "10s").Should(Succeed(), "read join token from %s", ips[0])
	r.browser.AddSecrets(joinToken)

	port := "6443"
	if r.Product == "rke2" {
		port = "9345"
	}
	join := fmt.Sprintf("server: %q\ntoken: %q\n", "https://"+net.JoinHostPort(firstPrivateIP, port), joinToken)
	for i := 1; i < len(ips); i++ {
		kind := "agent"
		if rancher.IsServer(r.ImportNodes[i]) {
			kind = "server"
		}
		r.writeProductConfig(ips[i], join)
		_, err = onNode(ips[i], standaloneInstallCmd(r.Product, r.PreviousVersion, kind), 15*time.Minute)
		Expect(err).NotTo(HaveOccurred(), "join %s %s on %s", kind, r.PreviousVersion, ips[i])
	}
}

// writeProductConfig writes /etc/rancher/<product>/config.yaml over stdin so the join token stays off command lines.
func (r *RancherDay1) writeProductConfig(ip, content string) {
	dir := "/etc/rancher/" + r.Product
	_, err := resources.RunCommandOnNodeWithStdin(
		"sudo mkdir -p "+dir+" && sudo tee "+dir+"/config.yaml >/dev/null && sudo chmod 600 "+dir+"/config.yaml",
		ip, strings.NewReader(content))
	Expect(err).NotTo(HaveOccurred(), "write %s config on %s", r.Product, ip)
}

// CheckImportUpgradeDropdown screenshots the imported cluster's edit dropdown and checks the version under test is offered.
func (r *RancherDay1) CheckImportUpgradeDropdown() {
	r.requireImport()
	r.checkDropdown(clusterListPath+"/"+rancher.FleetNamespace+"/"+day1ImportCluster+"?mode=edit",
		importedSelect, "import-upgrade", r.Version)
}

// UpgradeImported moves the imported cluster to the version under test through Rancher's version management.
func (r *RancherDay1) UpgradeImported() {
	r.requireImport()
	field := r.Product + "Config" // k3sConfig or rke2Config on the management cluster

	// Rancher fills <product>Config once it has detected the distro; patching earlier would race it.
	Eventually(func() (string, error) {
		return rancherKubectl("get", "clusters.management.cattle.io", r.importID,
			"-o", "jsonpath={.spec."+field+".kubernetesVersion}")
	}, "10m", "15s").ShouldNot(BeEmpty(), "rancher never detected %s on %s", r.Product, day1ImportCluster)

	_, err := rancherKubectl("annotate", "--overwrite", "clusters.management.cattle.io", r.importID,
		"rancher.io/imported-cluster-version-management=true")
	Expect(err).NotTo(HaveOccurred(), "enable version management on %s", day1ImportCluster)
	patch := fmt.Sprintf(`{"spec":{%q:{"kubernetesVersion":%q}}}`, field, r.Version)
	_, err = rancherKubectl("patch", "clusters.management.cattle.io", r.importID, "--type=merge", "-p", patch)
	Expect(err).NotTo(HaveOccurred(), "request upgrade of %s to %s", day1ImportCluster, r.Version)

	r.waitClusterVersion(day1ImportCluster, r.importID, r.importKubectlIP, r.Version, len(r.ImportNodes), "40m")
	r.openAndShoot(clusterListPath, day1ImportCluster, "imported-upgraded-"+r.Version)
}

// CollectDebug saves Rancher, Kubernetes and node logs plus a failure screenshot for a failed spec.
func (r *RancherDay1) CollectDebug(spec string) {
	r.debugRuns++
	dir := filepath.Join(r.artifactsDir, "debug", fmt.Sprintf("%02d", r.debugRuns))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		resources.LogLevel("error", "create debug dir: %v", err)
		return
	}
	save := func(name, out string, err error) {
		if err != nil {
			out += "\nERROR: " + err.Error()
		}
		if werr := os.WriteFile(filepath.Join(dir, name), []byte(out), 0o600); werr != nil {
			resources.LogLevel("error", "write %s: %v", name, werr)
		}
	}

	save("spec.txt", spec, nil)
	rancherLogs, err := rancherKubectl("logs", "-n", "cattle-system", "-l", "app=rancher", "--tail=500", "--prefix")
	save("rancher.log", rancherLogs, err)
	for name, args := range map[string][]string{
		"provisioning-clusters.yaml": {"get", "clusters.provisioning.cattle.io", "-n", rancher.FleetNamespace, "-o", "yaml"},
		"management-clusters.yaml":   {"get", "clusters.management.cattle.io", "-o", "yaml"},
		"rkecontrolplanes.yaml":      {"get", "rkecontrolplanes.rke.cattle.io", "-n", rancher.FleetNamespace, "-o", "yaml"},
		"machines.txt":               {"get", "machines.cluster.x-k8s.io", "-n", rancher.FleetNamespace, "-o", "wide"},
		"etcdsnapshots.txt":          {"get", "etcdsnapshots.rke.cattle.io", "-n", rancher.FleetNamespace},
		"events.txt":                 {"get", "events", "-n", rancher.FleetNamespace, "--sort-by=.lastTimestamp"},
		"cattle-system-pods.txt":     {"get", "pods", "-n", "cattle-system", "-o", "wide"},
	} {
		out, kerr := rancherKubectl(args...)
		save(name, out, kerr)
	}

	r.saveNodeDebug(save)
	r.saveBrowserDebug(spec)

	lines := strings.Split(strings.TrimSpace(rancherLogs), "\n")
	resources.LogLevel("info", "Debug data for %q saved to %s; last rancher log lines:\n%s",
		spec, dir, strings.Join(lines[max(0, len(lines)-40):], "\n"))
}

func (r *RancherDay1) saveNodeDebug(save func(name, out string, err error)) {
	journal := fmt.Sprintf("sudo journalctl -u rancher-system-agent -u %[1]s -u %[1]s-server -u %[1]s-agent"+
		" -n 400 --no-pager", r.Product)
	for name, ip := range r.nodeIPs {
		out, err := onNode(ip, journal, time.Minute)
		save(name+"-journal.log", out, err)
	}

	agentLogs := downstreamKubectl(r.Product) + " logs -n cattle-system -l app=cattle-cluster-agent --tail=300"
	for label, ip := range map[string]string{"custom": r.customKubectlIP, "import": r.importKubectlIP} {
		if ip != "" {
			out, err := onNode(ip, agentLogs, time.Minute)
			save(label+"-cluster-agent.log", out, err)
		}
	}
}

func (r *RancherDay1) saveBrowserDebug(spec string) {
	if r.browser == nil {
		return
	}
	if path, err := r.browser.Screenshot("FAILED-" + spec); err != nil {
		resources.LogLevel("warn", "failure screenshot: %v", err)
	} else {
		resources.LogLevel("info", "Failure screenshot: %s", path)
	}
	if _, err := r.browser.SaveConsole(fmt.Sprintf("debug-%02d", r.debugRuns)); err != nil {
		resources.LogLevel("warn", "save browser console: %v", err)
	}
}

// Cleanup closes the browser and, when destroying, terminates the downstream EC2 nodes created by this run.
func (r *RancherDay1) Cleanup(destroy bool) {
	if r.browser != nil {
		r.browser.Close()
	}
	ids := aws.TrackedInstanceIDs()
	if !destroy || len(ids) == 0 {
		if len(ids) > 0 {
			resources.LogLevel("info", "Keeping downstream instances (destroy=false): %v", ids)
		}
		return
	}
	if r.awsClient == nil {
		resources.LogLevel("error", "no AWS client; downstream instances may leak: %v", ids)
		return
	}
	if err := r.awsClient.DeleteInstances(ids...); err != nil {
		resources.LogLevel("error", "terminate downstream instances %v: %v", ids, err)
	}
}

func (r *RancherDay1) requireAPI() {
	if r.api == nil || r.browser == nil {
		Skip("rancher session was not established")
	}
}

func (r *RancherDay1) requireCustom() {
	r.requireAPI()
	if !r.customReady {
		Skip("custom cluster was not provisioned")
	}
}

func (r *RancherDay1) requireImport() {
	r.requireAPI()
	if r.importID == "" {
		Skip("standalone cluster was not imported")
	}
}

func (r *RancherDay1) checkDropdown(path, selector, label, want string) {
	Expect(r.browser.Open(path, "Kubernetes Version", 3*time.Minute)).To(Succeed())
	options, err := r.browser.DropdownOptions(selector, want, time.Minute)
	Expect(err).NotTo(HaveOccurred())
	shot, err := r.browser.Screenshot(label + "-dropdown-" + want)
	Expect(err).NotTo(HaveOccurred())
	resources.LogLevel("info", "%s dropdown (%s): %v", label, shot, options)
	Expect(options).To(ContainElement(ContainSubstring(want)), "%s dropdown is missing %s", label, want)
}

func (r *RancherDay1) openAndShoot(path, waitText, name string) {
	Expect(r.browser.Open(path, waitText, 3*time.Minute)).To(Succeed())
	shot, err := r.browser.Screenshot(name)
	Expect(err).NotTo(HaveOccurred())
	resources.LogLevel("info", "Screenshot: %s", shot)
}

func createRancherCluster(name, version string) string {
	manifest, err := rancher.ClusterManifest(name, version)
	Expect(err).NotTo(HaveOccurred())
	f, err := os.CreateTemp("", name+"-*.json")
	Expect(err).NotTo(HaveOccurred())
	defer os.Remove(f.Name())
	_, err = f.WriteString(manifest)
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Close()).To(Succeed())

	_, err = rancherKubectl("apply", "-f", f.Name())
	Expect(err).NotTo(HaveOccurred(), "create rancher cluster %s", name)

	var id string
	Eventually(func() string {
		id, _ = rancherKubectl("get", "clusters.provisioning.cattle.io", "-n", rancher.FleetNamespace, name,
			"-o", "jsonpath={.status.clusterName}")
		return id
	}, "5m", "10s").ShouldNot(BeEmpty(), "rancher never assigned a management cluster to %s", name)
	resources.LogLevel("info", "Rancher cluster %s is %s", name, id)

	return id
}

func (r *RancherDay1) registrationToken(clusterID string) *rancher.RegistrationToken {
	var token *rancher.RegistrationToken
	Eventually(func() error {
		raw, err := rancherKubectl("get", "clusterregistrationtokens.management.cattle.io", "-n", clusterID, "-o", "json")
		if err != nil {
			return err
		}
		token, err = rancher.ParseRegistrationToken(raw)

		return err
	}, "5m", "10s").Should(Succeed(), "registration token for %s", clusterID)
	r.browser.AddSecrets(token.Token)

	return token
}

// createNodes launches one EC2 instance per suffix and returns public and private IPs in suffix order.
func (r *RancherDay1) createNodes(suffixes ...string) (ips, privateIPs []string) {
	if r.awsClient == nil {
		client, err := aws.AddClient(r.cluster)
		Expect(err).NotTo(HaveOccurred(), "create AWS client for downstream nodes")
		Expect(client.UseInstanceSSHKey(context.Background(), r.cluster.ServerIPs[0])).To(Succeed())
		r.awsClient = client
	}

	for _, suffix := range suffixes {
		// One call per node: CreateInstances returns IPs in completion order, not name order.
		public, private, _, err := r.awsClient.CreateInstances(r.ResourceName + "-" + suffix)
		Expect(err).NotTo(HaveOccurred(), "create %s node", suffix)
		ips, privateIPs = append(ips, public[0]), append(privateIPs, private[0])
		r.nodeIPs[suffix] = public[0]
	}
	for _, ip := range ips {
		Expect(resources.WaitForSSHReadyWithTimeout(ip, 10*time.Minute)).To(Succeed())
	}

	return ips, privateIPs
}

func (r *RancherDay1) waitClusterVersion(name, id, kubectlIP, version string, nodes int, timeout string) {
	Eventually(func(g Gomega) {
		ready, err := rancherKubectl("get", "clusters.provisioning.cattle.io", "-n", rancher.FleetNamespace, name,
			"-o", "jsonpath={.status.ready}")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ready).To(Equal("true"), "%s not ready", name)

		reported, err := rancherKubectl("get", "clusters.management.cattle.io", id,
			"-o", "jsonpath={.status.version.gitVersion}")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(reported).To(Equal(version), "rancher reports %s at %q", name, reported)

		kubelets, err := onNode(kubectlIP, downstreamKubectl(r.Product)+
			" get nodes -o jsonpath='{.items[*].status.nodeInfo.kubeletVersion}'", time.Minute)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.Fields(kubelets)).To(HaveLen(nodes), "%s node count", name)
		g.Expect(strings.Fields(kubelets)).To(HaveEach(Equal(version)), "%s nodes not all on %s", name, version)
	}, timeout, "20s").Should(Succeed(), "%s did not become ready on %s", name, version)
	resources.LogLevel("info", "%s is ready on %s with %d nodes", name, version, nodes)
}

// bumpGeneration sets spec.rkeConfig.<field> to body (holding one %d) with the next generation and returns it.
func bumpGeneration(field, body string) int {
	raw, err := rancherKubectl("get", "clusters.provisioning.cattle.io", "-n", rancher.FleetNamespace, day1CustomCluster,
		"-o", "jsonpath={.spec.rkeConfig."+field+".generation}")
	Expect(err).NotTo(HaveOccurred())
	current := 0
	if strings.TrimSpace(raw) != "" {
		current, err = strconv.Atoi(strings.TrimSpace(raw))
		Expect(err).NotTo(HaveOccurred(), "parse %s generation %q", field, raw)
	}

	gen := current + 1
	patch := fmt.Sprintf(`{"spec":{"rkeConfig":{%q:`+body+`}}}`, field, gen)
	_, err = rancherKubectl("patch", "clusters.provisioning.cattle.io", "-n", rancher.FleetNamespace, day1CustomCluster,
		"--type=merge", "-p", patch)
	Expect(err).NotTo(HaveOccurred(), "request %s generation %d", field, gen)

	return gen
}

func waitControlPlane(what, timeout string, done func(*rancher.ControlPlane) bool) {
	Eventually(func(g Gomega) {
		raw, err := rancherKubectl("get", "rkecontrolplanes.rke.cattle.io", "-n", rancher.FleetNamespace,
			day1CustomCluster, "-o", "json")
		g.Expect(err).NotTo(HaveOccurred())
		cp, err := rancher.ParseControlPlane(raw)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(done(cp)).To(BeTrue(), "%s not finished: create=%s restore=%s ready=%v", what,
			cp.Status.ETCDSnapshotCreatePhase, cp.Status.ETCDSnapshotRestorePhase, cp.Ready())
	}, timeout, "20s").Should(Succeed(), "%s on %s", what, day1CustomCluster)
}

func listSnapshots() string {
	raw, err := rancherKubectl("get", "etcdsnapshots.rke.cattle.io", "-n", rancher.FleetNamespace,
		"-l", "rke.cattle.io/cluster-name="+day1CustomCluster, "-o", "json")
	Expect(err).NotTo(HaveOccurred(), "list etcd snapshots")

	return raw
}

func onNode(ip, cmd string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return resources.RunCommandOnNodeContext(ctx, cmd, ip)
}

// rancherKubectl runs kubectl against the Rancher local cluster without logging arguments or output.
func rancherKubectl(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	args = append([]string{"--kubeconfig=" + resources.KubeConfigFile}, args...)
	out, err := resources.RunHostArgsQuietContext(ctx, "kubectl", args...)

	return strings.TrimSpace(out), err
}

func setRancherSetting(name, value string) {
	patch, err := json.Marshal(map[string]string{"value": value})
	Expect(err).NotTo(HaveOccurred())
	_, err = rancherKubectl("patch", "settings.management.cattle.io", name, "--type=merge", "-p", string(patch))
	Expect(err).NotTo(HaveOccurred(), "set rancher setting %s", name)
}

// downstreamKubectl is a server node's own kubectl; sudo's secure_path may lack the product bin dirs.
func downstreamKubectl(product string) string {
	return fmt.Sprintf("sudo env PATH=/usr/local/bin:/opt/bin:/var/lib/rancher/%[1]s/bin:/usr/bin:/bin "+
		"kubectl --kubeconfig /etc/rancher/%[1]s/%[1]s.yaml", product)
}

// standaloneInstallCmd installs a server or agent that reads config.yaml; version is validated by rancher.ValidateVersion.
func standaloneInstallCmd(product, version, kind string) string {
	if product == "rke2" {
		return "curl -sfL https://get.rke2.io | sudo INSTALL_RKE2_VERSION='" + version + "' INSTALL_RKE2_TYPE='" + kind +
			"' sh - && sudo systemctl enable --now rke2-" + kind
	}

	return "curl -sfL https://get.k3s.io | sudo INSTALL_K3S_VERSION='" + version + "' INSTALL_K3S_EXEC='" + kind + "' sh -"
}
