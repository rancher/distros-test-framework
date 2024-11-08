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

func setConfigs() {
	awsConfig = shared.AwsConfig{
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
	}

}

func TestClusterRestoreS3(
	cluster *shared.Cluster,
	applyWorkload bool,
	flags *customflag.FlagConfig,
) {
	setConfigs()

	product := cluster.Config.Product
	_, version, err := shared.Product()
	Expect(err).NotTo(HaveOccurred())
	versionStr := fmt.Sprintf("%s version ", product)
	versionCleanUp := strings.TrimPrefix(version, versionStr)
	endChar := strings.Index(versionCleanUp, "(")
	versionClean := versionCleanUp[:endChar]

	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", product+"-extra-metadata.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "configmap failed to create")
	}

	shared.LogLevel("info", "%s-extra-metadata configmap successfully added", product)

	testTakeS3Snapshot(
		cluster,
		flags,
		true,
	)
	shared.LogLevel("info", "successfully completed s3 snapshot save")
	testS3SnapshotSave(
		cluster,
		flags,
	)
	shared.LogLevel("info", "successfully validated s3 snapshot save in s3")
	onDemandPath, onDemandPathErr := shared.FetchSnapshotOnDemandPath(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(onDemandPathErr).NotTo(HaveOccurred())

	clusterToken, clusterTokenErr := shared.FetchToken(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(clusterTokenErr).NotTo(HaveOccurred())

	resourceName := os.Getenv("resource_name")
	ec2, err := aws.AddClient(cluster)
	Expect(err).NotTo(HaveOccurred(), "error adding aws nodes: %s", err)

	stopServerInstances(cluster, ec2)

	stopAgentInstance(cluster, ec2)

	// oldLeadServerIP := cluster.ServerIPs[0]

	// create new server.
	var serverName []string

	serverName = append(serverName, fmt.Sprintf("%s-server-fresh", resourceName))

	externalServerIP, _, _, createErr :=
		ec2.CreateInstances(serverName...)
	Expect(createErr).NotTo(HaveOccurred(), createErr)

	shared.LogLevel("info", "Created server public ip: %s",
		externalServerIP[0])

	newServerIP := externalServerIP

	setConfigFile(product, newServerIP[0])
	shared.LogLevel("info", "config.yaml successfully created and copied to: /etc/rancher/%s/", product)

	installProduct(
		cluster,
		newServerIP[0],
		versionClean,
	)
	shared.LogLevel("info", "%s successfully installed on server: %s", product, newServerIP)

	testRestoreS3Snapshot(
		cluster,
		onDemandPath,
		clusterToken,
		newServerIP[0],
		flags,
	)
	shared.LogLevel("info", "cluster restore successful. Waiting 120 seconds for cluster "+
		"to complete background processes after restore.")
	time.Sleep(120 * time.Second)

	enableAndStartService(
		cluster,
		newServerIP[0],
	)
	shared.LogLevel("info", "%s service successfully enabled", product)

	// if product == "rke2" {
	// 	exportKubectl(newServerIP[0])
	// 	shared.LogLevel("info", "kubectl commands successfully exported")
	// }

	fmt.Println("Server IP: ", newServerIP[0])
	fmt.Println("Server Name: ", serverName[0])

	copyCmd := fmt.Sprintf("cp /tmp/%s_kubeconfig /tmp/%s_kubeconfig", resourceName, serverName[0])

	_, copyCmdErr := shared.RunCommandHost(copyCmd)
	Expect(copyCmdErr).NotTo(HaveOccurred())

	_, err = shared.UpdateKubeConfig(newServerIP[0], serverName[0], product)
	Expect(err).NotTo(HaveOccurred())

	postValidationS3(cluster, newServerIP[0])
	shared.LogLevel("info", "%s server successfully validated post restore", product)

}

func postValidationS3(cluster *shared.Cluster, newServerIP string) {
	kubeconfigFlagRemotePath := fmt.Sprintf("/etc/rancher/%s/%s.yaml", cluster.Config.Product, cluster.Config.Product)
	kubeconfigFlagRemote := "--kubeconfig=" + kubeconfigFlagRemotePath

	// fmt.Println("DEBUG 1A: ", newServerIP)

	// kubectlLocationCmd := "which kubectl"
	// fmt.Println("PRINTING KUBECTL: ", kubectlLocationCmd)

	// kubectlLocationCmd2, findErr := shared.FindPath("kubectl", newServerIP)
	// Expect(findErr).NotTo(HaveOccurred())
	// shared.LogLevel("printing kubectl path via shared.FindPath: %s", kubectlLocationCmd2)
	// fmt.Printf("Location of kubectl: %s", kubectlLocationCmd2)

	// kubectlLocationRes, kubectlLocationErr := shared.RunCommandOnNode(kubectlLocationCmd, newServerIP)
	// Expect(kubectlLocationErr).NotTo(HaveOccurred())
	// fmt.Printf("Location of kubectl: %s", kubectlLocationRes)

	shared.PrintClusterState()

	getNodesPodsCmd := fmt.Sprintf("/var/lib/rancher/%s/bin/kubectl get nodes,pods -A -o wide %s", cluster.Config.Product, kubeconfigFlagRemote)
	shared.LogLevel("Running %s on ip: %s", getNodesPodsCmd, newServerIP)
	// validatePodsCmd := "kubectl get pods " + kubeconfigFlagRemote
	// time.Sleep(240 * time.Second)
	_, nodesPodsErr := shared.RunCommandOnNode(getNodesPodsCmd, newServerIP)
	Expect(nodesPodsErr).NotTo(HaveOccurred())
	// fmt.Println("Response: ", nodesPodsRes)
	// fmt.Println("Error: ", nodesPodsErr.Error())
	// validatePodsRes, validatePodsErr := shared.RunCommandOnNode(validatePodsCmd, newServerIP)
	// fmt.Println("Response: ", validatePodsRes)
	// fmt.Println("Error: ", validatePodsErr.Error())

	// if header == name containsSubstring("nodeport") & header == status == ContainsSubstring("Completed/Running")
}

// func TestPostRestoreS3() {

// }

func testS3SnapshotSave(cluster *shared.Cluster, flags *customflag.FlagConfig) {

	s3, err := aws.AddClient(cluster)
	Expect(err).NotTo(HaveOccurred(), "error creating s3 client: %s", err)

	s3.GetObjects(flags)
}

func testTakeS3Snapshot(
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

	takeSnapshotRes, takeSnapshotErr := shared.RunCommandOnNode(takeSnapshotCmd, cluster.ServerIPs[0])
	Expect(takeSnapshotErr).NotTo(HaveOccurred())
	Expect(takeSnapshotRes).To(ContainSubstring("Snapshot on-demand"))

	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", "daemonset.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Daemonset manifest not deployed")
	}
}

func stopServerInstances(cluster *shared.Cluster, ec2 *aws.Client) {

	var serverInstanceIDs string
	var serverInstanceIDsErr error
	for i := 0; i < len(cluster.ServerIPs); i++ {
		serverInstanceIDs, serverInstanceIDsErr = ec2.GetInstanceIDByIP(cluster.ServerIPs[i])
		Expect(serverInstanceIDsErr).NotTo(HaveOccurred())
		fmt.Println("Old Server Instance IDs: ", serverInstanceIDs)
		ec2.StopInstance(serverInstanceIDs)
		Expect(serverInstanceIDsErr).NotTo(HaveOccurred())
	}

}

func stopAgentInstance(cluster *shared.Cluster, ec2 *aws.Client) {

	for i := 0; i < len(cluster.AgentIPs); i++ {
		agentInstanceIDs, agentInstanceIDsErr := ec2.GetInstanceIDByIP(cluster.AgentIPs[i])
		Expect(agentInstanceIDsErr).NotTo(HaveOccurred())
		fmt.Println(agentInstanceIDs)
		ec2.StopInstance(agentInstanceIDs)
		Expect(agentInstanceIDsErr).NotTo(HaveOccurred())
	}

}

func installProduct(
	cluster *shared.Cluster,
	newClusterIP string,
	version string,
) {

	if cluster.Config.Product == "k3s" {
		installCmd := fmt.Sprintf("curl -sfL https://get.k3s.io/ | sudo INSTALL_K3S_VERSION=%s "+
			"INSTALL_K3S_SKIP_ENABLE=true sh -", version)
		_, installCmdErr := shared.RunCommandOnNode(installCmd, newClusterIP)
		Expect(installCmdErr).NotTo(HaveOccurred())
	} else if cluster.Config.Product == "rke2" {
		installCmd := fmt.Sprintf("curl -sfL https://get.rke2.io | sudo INSTALL_RKE2_VERSION=%s sh -", version)
		_, installCmdErr := shared.RunCommandOnNode(installCmd, newClusterIP)
		Expect(installCmdErr).NotTo(HaveOccurred())
	} else {
		shared.LogLevel("error", "unsupported product")
	}
}

func testRestoreS3Snapshot(
	cluster *shared.Cluster,
	onDemandPath,
	token string,
	newClusterIP string,
	flags *customflag.FlagConfig,
) {
	productLocationCmd, findErr := shared.FindPath(cluster.Config.Product, newClusterIP)
	Expect(findErr).NotTo(HaveOccurred())
	restoreCmd := fmt.Sprintf("sudo %s server --cluster-reset --etcd-s3 --cluster-reset-restore-path=%s"+
		" --etcd-s3-bucket=%s --etcd-s3-folder=%s --etcd-s3-region=%s --etcd-s3-access-key=%s"+
		" --etcd-s3-secret-key=%s --token=%s", productLocationCmd, onDemandPath, flags.S3Flags.Bucket,
		flags.S3Flags.Folder, cluster.Aws.Region, awsConfig.AccessKeyID, awsConfig.SecretAccessKey, token)
	if cluster.Config.Product == "k3s" {
		restoreCmdRes, resetCmdErr := shared.RunCommandOnNode(restoreCmd, newClusterIP)
		Expect(resetCmdErr).NotTo(HaveOccurred())
		Expect(restoreCmdRes).To(ContainSubstring("Managed etcd cluster"))
		Expect(restoreCmdRes).To(ContainSubstring("has been reset"))
	} else if cluster.Config.Product == "rke2" {
		_, restoreCmdErr := shared.RunCommandOnNode(restoreCmd, newClusterIP)
		Expect(restoreCmdErr).To(HaveOccurred())
		Expect(restoreCmdErr.Error()).To(ContainSubstring("Managed etcd cluster"))
		Expect(restoreCmdErr.Error()).To(ContainSubstring("has been reset"))
	}
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
	// fmt.Println("START SERVICE OUT: ", startServiceCmdErr.Error())
	shared.LogLevel("info", "Starting service, waiting for service to complete background processes.")
	time.Sleep(600 * time.Second)
	Expect(startServiceCmdErr).NotTo(HaveOccurred())
	statusServiceCmdRes, statusServiceCmdErr := shared.ManageService(cluster.Config.Product, "status", "server",
		[]string{newClusterIP})
	fmt.Println("STATUS SERVICE OUT: ", statusServiceCmdRes)
	fmt.Println("STATUS SERVICE ERR: ", statusServiceCmdErr)
	Expect(statusServiceCmdErr).NotTo(HaveOccurred())

	// Expect(statusServiceCmdRes).To(SatisfyAll(ContainSubstring("enabled"), ContainSubstring("active")))
}

// func exportKubectl(newClusterIP string) {
// 	//update data directory for rpm installs (rhel systems)
// 	exportCmd := fmt.Sprintf("sudo cat <<EOF >>.bashrc\n" +
// 		"export KUBECONFIG=/etc/rancher/rke2/rke2.yaml PATH=$PATH:/var/lib/rancher/rke2/bin:/opt/rke2/bin " +
// 		"CRI_CONFIG_FILE=/var/lib/rancher/rke2/agent/etc/crictl.yaml && \n" +
// 		"alias k=kubectl\n" +
// 		"EOF")

// 	sourceCmd := "source .bashrc"

// 	_, exportCmdErr := shared.RunCommandOnNode(exportCmd, newClusterIP)
// 	_, sourceCmdErr := shared.RunCommandOnNode(sourceCmd, newClusterIP)
// 	Expect(sourceCmdErr).NotTo(HaveOccurred())
// 	exportCmd = fmt.Printf("echo $PATH")

// 	fmt.Printf("DEBUG 0: PATH: ")
// 	_, exportCmdErr := shared.RunCommandOnNode(exportCmd, newClusterIP)
// 	exportCmd = fmt.Printf("ls /var/lib/rancher/rke2/bin")

// 	fmt.Printf("DEBUG 0: PATH: ")
// 	_, exportCmdErr := shared.RunCommandOnNode(exportCmd, newClusterIP)
// 	Expect(exportCmdErr).NotTo(HaveOccurred())

// }

func setConfigFile(product string, newClusterIP string) {
	createConfigFileCmd := fmt.Sprintf("sudo cat <<EOF >>config.yaml\n"+
		"write-kubeconfig-mode: 644\n"+
		"node-external-ip: %s\n"+
		"cluster-init: true\n"+
		"EOF", newClusterIP)

	path := fmt.Sprintf("/etc/rancher/%s/", product)
	mkdirCmd := fmt.Sprintf("sudo mkdir -p %s", path)
	copyConfigFileCmd := fmt.Sprintf("sudo cp config.yaml %s", path)

	_, createConfigFileCmdErr := shared.RunCommandOnNode(createConfigFileCmd, newClusterIP)
	Expect(createConfigFileCmdErr).NotTo(HaveOccurred())

	_, mkdirCmdErr := shared.RunCommandOnNode(mkdirCmd, newClusterIP)
	Expect(mkdirCmdErr).NotTo(HaveOccurred())

	_, copyConfigFileCmdErr := shared.RunCommandOnNode(copyConfigFileCmd, newClusterIP)
	Expect(copyConfigFileCmdErr).NotTo(HaveOccurred())

}
