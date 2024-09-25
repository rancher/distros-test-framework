package testcase

import (
	"fmt"
	"os"

	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

func TestClusterRestoreS3(
	cluster *shared.Cluster,
	applyWorkload,
	deleteWorkload bool,
) {
	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", cluster.Config.Product+"-extra-metadata.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "configmap failed to create")
	}

	shared.LogLevel("info", "%s-extra-metadata configmap successfully added", cluster.Config.Product)

	s3Bucket := os.Getenv("S3_BUCKET")
	s3Folder := os.Getenv("S3_FOLDER")
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	s3Region := ""

	takeS3Snapshot(
		cluster,
		s3Bucket,
		s3Folder,
		s3Region,
		accessKeyID,
		secretAccessKey,
		true,
		false,
	)

	onDemandPath, onDemandPathErr := shared.FetchSnapshotOnDemandPath(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(onDemandPathErr).NotTo(HaveOccurred())

	fmt.Println("\non-demand-path: ", onDemandPath)

	clusterToken, clusterTokenErr := shared.FetchToken(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(clusterTokenErr).NotTo(HaveOccurred())

	fmt.Println("\ntoken: ", clusterToken)

	// for i := 0; i < len(cluster.ServerIPs); i++ {
	// 	shared.LogLevel("info", "stopping server instances: %s", cluster.ServerIPs[i])
	// }
	stopServerInstances(cluster)

	// shared.LogLevel("info", "stopping agent instance: %s", cluster.AgentIPs[0])

	stopAgentInstance(cluster)

	resourceName := os.Getenv("resource_name")
	awsDependencies, err := aws.AddEC2Client(cluster)
	Expect(err).NotTo(HaveOccurred(), "error adding aws nodes: %s", err)

	// create server names.
	var serverName []string

	serverName = append(serverName, fmt.Sprintf("%s-server-fresh", resourceName))

	externalServerIP, _, _, createErr :=
		awsDependencies.CreateInstances(serverName...)
	Expect(createErr).NotTo(HaveOccurred(), createErr)

	shared.LogLevel("info", "Created server public ip: %s",
		externalServerIP)

	// createNewServer(externalServerIP)

	// how do I delete the instances, bring up a new instance and install K3s/RKE2 using what we currently have?
	// shared.LogLevel("info", "running cluster reset on server %s\n", cluster.ServerIPs[0])
	// restoreS3Snapshot(
	// 	cluster,
	// 	s3Bucket,
	// 	s3Folder,
	// 	s3Region,
	// 	onDemandPath,
	// 	accessKeyID,
	// 	secretAccessKey,
	// 	clusterToken,
	// )
}

func TestS3SnapshotSave(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	s3Config := shared.AwsS3Config{
		AccessKey: os.Getenv("access_key"),
		Region: os.Getenv("region"),
		Bucket: flags.S3Flags.Bucket,
		Folder: flags.S3Flags.Folder,
	}
	s3Client, err := aws.AddS3Client(s3Config)
	Expect(err).NotTo(HaveOccurred(), "error creating s3 client: %s", err)

	s3Client.GetObjects(s3Config)
}

// perform snapshot and list snapshot commands -- deploy workloads after snapshot [apply workload]
func takeS3Snapshot(
	cluster *shared.Cluster,
	s3Bucket,
	s3Folder,
	s3Region,
	accessKeyID,
	secretAccessKey string,
	applyWorkload,
	deleteWorkload bool,
) {
	productLocationCmd, findErr := shared.FindPath(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(findErr).NotTo(HaveOccurred())

	takeSnapshotCmd := fmt.Sprintf("sudo %s etcd-snapshot save --s3 --s3-bucket=%s "+
		"--s3-folder=%s --s3-region=%s --s3-access-key=%s --s3-secret-key=%s",
		productLocationCmd, s3Bucket, s3Folder, s3Region, accessKeyID, secretAccessKey)

	takeSnapshotRes, takeSnapshotErr := shared.RunCommandOnNode(takeSnapshotCmd, cluster.ServerIPs[0])
	Expect(takeSnapshotRes).To(ContainSubstring("Snapshot on-demand"))
	Expect(takeSnapshotErr).NotTo(HaveOccurred())

	// add validation that the s3 folder has been created

	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", "daemonset.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Daemonset manifest not deployed")
	}

	// diff command -- comparison of outputs []

}

func stopServerInstances(cluster *shared.Cluster) {

	awsDependencies, err := aws.AddEC2Client(cluster)
	Expect(err).NotTo(HaveOccurred())
	// stop server Instances
	for i := 0; i < len(cluster.ServerIPs); i++ {
		serverInstanceIDs, serverInstanceIDsErr := awsDependencies.GetInstanceIDByIP(cluster.ServerIPs[i])
		Expect(serverInstanceIDsErr).NotTo(HaveOccurred())
		fmt.Println(serverInstanceIDs)
		awsDependencies.StopInstance(serverInstanceIDs)
	}

}

func stopAgentInstance(cluster *shared.Cluster) {
	// stop agent Instances
	awsDependencies, err := aws.AddEC2Client(cluster)
	Expect(err).NotTo(HaveOccurred())

	agentInstanceIDs, agentInstanceIDsErr := awsDependencies.GetInstanceIDByIP(cluster.AgentIPs[0])
	Expect(agentInstanceIDsErr).NotTo(HaveOccurred())
	fmt.Println(agentInstanceIDs)
	awsDependencies.StopInstance(agentInstanceIDs)
}

func createNewServer(cluster *shared.Cluster) (externalServerIP []string) {

	resourceName := os.Getenv("resource_name")
	awsDependencies, err := aws.AddEC2Client(cluster)
	Expect(err).NotTo(HaveOccurred(), "error adding aws nodes: %s", err)

	// create server names.
	var serverName []string
	serverName = append(serverName, resourceName+"-server1-new")

	externalServerIP, _, _, createErr :=
		awsDependencies.CreateInstances(serverName...)
	Expect(createErr).NotTo(HaveOccurred(), createErr)

	shared.LogLevel("info", "Created server public ip: %s",
		externalServerIP)

	return externalServerIP
}

// func installProduct(
// 	cluster *share.cluster,
// 	token string,
// 	externalServerIP string,
// ) {
// 	version := cluster.Config.Version

// 	if cluster.Config.Product == "k3s" {
// 		installCmd := fmt.Sprintf("curl -sfL https://get.k3s.io/ | sudo INSTALL_K3S_VERSION=%s INSTALL_K3S_SKIP_ENABLE=true sh -", version)
// 		_, installCmdErr := shared.RunCommandOnNode(installCmd, externalServerIP)
// 		Expect(installCmdErr).NotTo(HaveOccurred())
// 	} else {
// 		installCmd := fmt.Sprintf("curl -sfL https://get.rke2.io | sudo INSTALL_RKE2_VERSION=%s sh -", version)
// 	}
// }

func installProduct(cluster *shared.Cluster, token string, externalServerIP string) {

}

func restoreS3Snapshot(
	cluster *shared.Cluster,
	s3Bucket,
	s3Folder,
	s3Region,
	onDemandPath,
	accessKeyID,
	secretAccessKey,
	token string,
) {
	// var path string
	productLocationCmd, findErr := shared.FindPath(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(findErr).NotTo(HaveOccurred())
	resetCmd := fmt.Sprintf("sudo %s server --cluster-reset --etcd-s3 --cluster-reset-restore-path=%s"+
		"--etcd-s3-bucket=%s --etcd-s3-folder=%s --etcd-s3-region=%s --etcd-s3-access-key=%s"+
		"--etcd-s3-secret-key=%s --token=%s", productLocationCmd, onDemandPath, s3Bucket, s3Folder, s3Region, accessKeyID,
		secretAccessKey, token)
	resetRes, resetCmdErr := shared.RunCommandOnNode(resetCmd, cluster.ServerIPs[0])
	Expect(resetCmdErr).NotTo(HaveOccurred())
	Expect(resetRes).To(ContainSubstring("Managed etcd cluster"))
	Expect(resetRes).To(ContainSubstring("has been reset"))
}

// func deleteOldNodes() {

// }
