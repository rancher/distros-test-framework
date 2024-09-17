package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/shared"

	"os"

	. "github.com/onsi/gomega"
)

func TestClusterResetRestoreS3Snapshot(cluster *shared.Cluster, applyWorkload, deleteWorkload bool) {
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
	s3Region := cluster.AwsEc2.Region

	takeS3Snapshot(cluster, s3Bucket, s3Folder, s3Region, accessKeyID, secretAccessKey, true, false)

	onDemandPathCmd := fmt.Sprintf("sudo ls /var/lib/rancher/%s/server/db/snapshots", cluster.Config.Product)
	onDemandPath, _ := shared.RunCommandOnNode(onDemandPathCmd, cluster.ServerIPs[0])

	fmt.Println("\non-demand-path: ", onDemandPath)

	clusterTokenCmd := fmt.Sprintf("sudo cat /var/lib/rancher/%s/server/token", cluster.Config.Product)
	clusterToken, _ := shared.RunCommandOnNode(clusterTokenCmd, cluster.ServerIPs[0])

	fmt.Println("\ntoken: ", clusterToken)

	// stopInstances()
	// create fresh new VM and install K3s/RKE2 using RunCommandOnNode
	// how do I delete the instances, bring up a new instance and install K3s/RKE2 using what we currently have?
	shared.LogLevel("info", "running cluster reset on server %s\n", cluster.ServerIPs[0])
	restoreS3Snapshot(cluster, s3Bucket, s3Folder, s3Region, onDemandPath, accessKeyID, secretAccessKey, clusterToken)

}

// perform snapshot and list snapshot commands -- deploy workloads after snapshot [apply workload]
func takeS3Snapshot(cluster *shared.Cluster, s3Bucket, s3Folder, s3Region, accessKeyID, secretAccessKey string,
	applyWorkload, deleteWorkload bool) {
	productLocationCmd, findErr := shared.FindPath(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(findErr).NotTo(HaveOccurred())

	takeSnapshotCmd := fmt.Sprintf("sudo %s etcd-snapshot save --s3 --s3-bucket=%s "+
		"--s3-folder=%s --s3-region=%s --s3-access-key=%s --s3-secret-key=%s",
		productLocationCmd, s3Bucket, s3Folder, s3Region, accessKeyID, secretAccessKey)

	takeSnapshotRes, takeSnapshotErr := shared.RunCommandOnNode(takeSnapshotCmd, cluster.ServerIPs[0])
	Expect(takeSnapshotRes).To(ContainSubstring("Snapshot on-demand"))
	Expect(takeSnapshotErr).NotTo(HaveOccurred())

	var workloadErr error
	if applyWorkload {
		workloadErr = shared.ManageWorkload("apply", "daemonset.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Daemonset manifest not deployed")
	}

	// diff command -- comparison of outputs []

}

func restoreS3Snapshot(cluster *shared.Cluster, s3Bucket, s3Folder, s3Region, onDemandPath, accessKeyID, secretAccessKey, token string) {
	// var path string
	productLocationCmd, findErr := shared.FindPath(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(findErr).NotTo(HaveOccurred())
	resetCmd := fmt.Sprintf("sudo %s server --cluster-reset --etcd-s3 --cluster-reset-restore-path=%s"+
		"--etcd-s3-bucket=%s --etcd-s3-folder=%s --etcd-s3-region=us-east-2 --etcd-s3-access-key=%s"+
		"--etcd-s3-secret-key=%s --token=%s", productLocationCmd, s3Bucket, s3Folder, s3Region, accessKeyID,
		secretAccessKey)
	resetRes, resetCmdErr := shared.RunCommandOnNode(resetCmd, cluster.ServerIPs[0])
	Expect(resetCmdErr).NotTo(HaveOccurred())
	Expect(resetRes).To(ContainSubstring("Managed etcd cluster"))
	Expect(resetRes).To(ContainSubstring("has been reset"))
}

// make sure the workload you deployed after the snapshot isn't present after the restore snapshot

// func installProduct() {

// }

// func deleteOldNodes() {

// }
