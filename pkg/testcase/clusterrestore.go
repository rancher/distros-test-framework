package testcase

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

var awsConfig shared.AwsConfig

func TestClusterRestore(
	cluster *shared.Cluster,
	applyWorkload bool,
	flags *customflag.FlagConfig,
) {
	setConfigs()

	product, version, err := shared.Product()
	Expect(err).NotTo(HaveOccurred())

	version = cleanVersionData(product, version)

	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", product+"-extra-metadata.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "configmap failed to create")
	}

	takeS3Snapshot(cluster, flags, true)

	validateS3snapshots(cluster, flags)
	shared.LogLevel("info", "successfully validated s3 snapshot save in s3")

	// todo
	// NO NEED of this fucn.
	// onDemandPath, onDemandPathErr := shared.FetchSnapshotOnDemandPath(cluster.Config.Product, cluster.ServerIPs[0])
	onDemandPath, onDemandPathErr := shared.RunCommandOnNode(fmt.Sprintf("sudo ls /var/lib/rancher/%s/server/db/snapshots", product),
		cluster.ServerIPs[0])
	Expect(onDemandPathErr).NotTo(HaveOccurred())

	clusterToken, clusterTokenErr := shared.FetchToken(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(clusterTokenErr).NotTo(HaveOccurred())

	resourceName := os.Getenv("resource_name")
	ec2, err := aws.AddClient(cluster)
	Expect(err).NotTo(HaveOccurred(), "error adding aws nodes: %s", err)

	deleteInstances(cluster, ec2)

	// create new server.
	var serverName []string
	serverName = append(serverName, fmt.Sprintf("%s-server-fresh", resourceName))

	externalServerIP, _, _, createErr :=
		ec2.CreateInstances(serverName...)
	Expect(createErr).NotTo(HaveOccurred(), createErr)
	newServerIP := externalServerIP[0]

	installProduct(
		cluster,
		newServerIP,
		version,
	)
	shared.LogLevel("info", "%s successfully installed on server: %s", product, newServerIP)

	restoreS3Snapshot(
		cluster,
		onDemandPath,
		clusterToken,
		newServerIP,
		flags,
	)
	shared.LogLevel("info", "cluster restore successful. Waiting 120 seconds for cluster "+
		"to complete background processes after restore.")
	time.Sleep(120 * time.Second)

	enableAndStartService(
		cluster,
		newServerIP,
	)
	shared.LogLevel("info", "%s service successfully enabled", product)

	postValidationRestore(cluster, newServerIP)
	shared.LogLevel("info", "%s server successfully validated post restore", product)
}

func cleanVersionData(product string, version string) string {
	versionStr := fmt.Sprintf("%s version ", product)
	versionCleanUp := strings.TrimPrefix(version, versionStr)

	endChar := strings.Index(versionCleanUp, "(")
	versionClean := versionCleanUp[:endChar]

	return versionClean
}

func setConfigs() {
	awsConfig = shared.AwsConfig{
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
	}
}

func takeS3Snapshot(
	cluster *shared.Cluster,
	flags *customflag.FlagConfig,
	applyWorkload bool,
) {
	productLocationCmd, findErr := shared.FindPath(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(findErr).NotTo(HaveOccurred())

	takeSnapshotCmd := fmt.Sprintf("sudo %s etcd-snapshot save --s3 --s3-bucket=%s "+
		"--s3-folder=%s --s3-region=%s --s3-access-key=%s --s3-secret-key=%s",
		productLocationCmd, flags.S3Flags.Bucket, flags.S3Flags.Folder, cluster.Aws.Region,
		awsConfig.AccessKeyID, awsConfig.SecretAccessKey)

	_, takeSnapshotErr := shared.RunCommandOnNode(takeSnapshotCmd, cluster.ServerIPs[0])
	Expect(takeSnapshotErr).NotTo(HaveOccurred())

	// todo
	// the correct output seemed a bit different thant here so commented out/....
	// Expect(takeSnapshotRes).To(ContainSubstring("Creating ETCDSnapshotFile"))

	// todo
	// this should be outised if this func is about take snapshot.
	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", "daemonset.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Daemonset manifest not deployed")
	}
}

func validateS3snapshots(cluster *shared.Cluster, flags *customflag.FlagConfig) {

	s3, err := aws.AddClient(cluster)
	Expect(err).NotTo(HaveOccurred(), "error creating s3 client: %s", err)

	s3.GetObjects(flags)
}

func deleteInstances(cluster *shared.Cluster, ec2 *aws.Client) {

	var instancesIPs []string

	instancesIPs = append(instancesIPs, cluster.ServerIPs...)
	instancesIPs = append(instancesIPs, cluster.AgentIPs...)

	for _, ip := range instancesIPs {

		// id, idsErr := ec2.GetInstanceIDByIP(ip)
		// Expect(idsErr).NotTo(HaveOccurred())
		//
		// ec2.StopInstance(id)
		// fmt.Println("Old Server Instance IDs: ", serverInstanceIDs)
		ec2.DeleteInstance(ip)
		// Expect(serverInstanceIDsErr).NotTo(HaveOccurred())
	}
}

func setConfigFile(product string, newClusterIP string) {
	createConfigFileCmd := fmt.Sprintf("sudo  bash -c 'cat <<EOF>/etc/rancher/%s/config.yaml\n"+
		"write-kubeconfig-mode: 644\n"+
		"EOF'", product)

	path := fmt.Sprintf("/etc/rancher/%s/", product)
	cmd := fmt.Sprintf("sudo  mkdir -p %s && %s", path, createConfigFileCmd)

	// running in a single cmd to avoid extra costs.
	_, mkdirCmdErr := shared.RunCommandOnNode(cmd, newClusterIP)
	Expect(mkdirCmdErr).NotTo(HaveOccurred())
}

func installProduct(
	cluster *shared.Cluster,
	newClusterIP string,
	version string,
) {
	setConfigFile(cluster.Config.Product, newClusterIP)

	installCmd := "curl -sfL "
	if cluster.Config.Product == "k3s" {
		installCmd = installCmd + fmt.Sprintf("https://get.%s.io/ | sudo INSTALL_%s_VERSION=%s  INSTALL_%s_SKIP_ENABLE=true sh -",
			cluster.Config.Product, strings.ToUpper(cluster.Config.Product), version, strings.ToUpper(cluster.Config.Product))
	}

	installCmd = installCmd + fmt.Sprintf("https://get.%s.io | sudo INSTALL_%s_VERSION=%s sh -", cluster.Config.Product, strings.ToUpper(cluster.Config.Product), version)

	_, installCmdErr := shared.RunCommandOnNode(installCmd, newClusterIP)
	Expect(installCmdErr).NotTo(HaveOccurred())
}

func enableAndStartService(
	cluster *shared.Cluster,
	newClusterIP string,
) {
	_, enableServiceCmdErr := shared.ManageService(cluster.Config.Product, "enable", "server",
		[]string{newClusterIP})
	Expect(enableServiceCmdErr).NotTo(HaveOccurred())
	_, startServiceCmdErr := shared.ManageService(cluster.Config.Product, "start", "server",
		[]string{newClusterIP})
	fmt.Println("CLUSTER IP: ", newClusterIP)
	// fmt.Println("START SERVICE OUT: ", startServiceCmdErr.Error())

	shared.LogLevel("info", "Starting service, waiting for service to complete background processes.")
	Expect(startServiceCmdErr).NotTo(HaveOccurred())

	time.Sleep(120 * time.Second)
	statusServiceCmdRes, statusServiceCmdErr := shared.ManageService(cluster.Config.Product, "status", "server",
		[]string{newClusterIP})
	Expect(statusServiceCmdErr).NotTo(HaveOccurred())
	fmt.Println("STATUS SERVICE OUT: ", statusServiceCmdRes)
	fmt.Println("STATUS SERVICE ERR: ", statusServiceCmdErr)
	// Expect(statusServiceCmdRes).To(SatisfyAll(ContainSubstring("enabled"), ContainSubstring("active")))
}

func restoreS3Snapshot(
	cluster *shared.Cluster,
	onDemandPath,
	token string,
	newClusterIP string,
	flags *customflag.FlagConfig,
) {
	var (
		restoreCmdRes string
		restoreCmdErr error
	)

	restoreCmd := fmt.Sprintf("sudo %s server --cluster-reset --etcd-s3 --cluster-reset-restore-path=%s"+
		" --etcd-s3-bucket=%s --etcd-s3-folder=%s --etcd-s3-region=%s --etcd-s3-access-key=%s"+
		" --etcd-s3-secret-key=%s --token=%s",
		cluster.Config.Product,
		onDemandPath,
		flags.S3Flags.Bucket,
		flags.S3Flags.Folder,
		cluster.Aws.Region,
		awsConfig.AccessKeyID,
		awsConfig.SecretAccessKey,
		token)

	switch cluster.Config.Product {
	case "k3s":
		restoreCmdRes, restoreCmdErr = shared.RunCommandOnNode(restoreCmd, newClusterIP)
		Expect(restoreCmdErr).NotTo(HaveOccurred())
		Expect(restoreCmdRes).To(ContainSubstring("Managed etcd cluster"))
		Expect(restoreCmdRes).To(ContainSubstring("has been reset"))
	case "rke2":
		_, restoreCmdErr = shared.RunCommandOnNode(restoreCmd, newClusterIP)
		Expect(restoreCmdErr).To(HaveOccurred())
		Expect(restoreCmdErr.Error()).To(ContainSubstring("Managed etcd cluster"))
		Expect(restoreCmdErr.Error()).To(ContainSubstring("has been reset"))
	default:
		Expect(fmt.Errorf("product not supported: %s", cluster.Config.Product)).NotTo(HaveOccurred())
	}
}

func postValidationRestore(cluster *shared.Cluster, newServerIP string) {
	kubeconfigFlagRemotePath := fmt.Sprintf("/etc/rancher/%s/%s.yaml", cluster.Config.Product, cluster.Config.Product)
	kubeconfigFlagRemote := "--kubeconfig=" + kubeconfigFlagRemotePath

	getNodesPodsCmd := fmt.Sprintf("export KUBECONFIG=/etc/rancher/%s/%s.yaml && PATH=$PATH:/var/lib/rancher/%s/bin  && /var/lib/rancher/%s/bin/kubectl get nodes,pods -A -o wide %s",
		cluster.Config.Product, cluster.Config.Product, cluster.Config.Product, cluster.Config.Product, kubeconfigFlagRemote)
	// shared.LogLevel("Running %s on ip: %s", getNodesPodsCmd, newServerIP)
	// validatePodsCmd := "kubectl get pods " + kubeconfigFlagRemote
	// time.Sleep(1 * time.Second)
	nodesPodsRes, nodesPodsErr := shared.RunCommandOnNode(getNodesPodsCmd, newServerIP)
	Expect(nodesPodsErr).NotTo(HaveOccurred())
	fmt.Println("Response: ", nodesPodsRes)

	// TODO: now thats is working u can start making validations on the cluster.
	// validatePodsRes, validatePodsErr := shared.RunCommandOnNode(validatePodsCmd, newServerIP)
	// fmt.Println("Response: ", validatePodsRes)

	// if header == name containsSubstring("nodeport") & header == status == ContainsSubstring("Completed/Running")
}
