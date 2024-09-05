package testcase

import (
	"fmt"

	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	"os"
	// . "github.com/onsi/gomega"
)

func TestClusterResetRestoreS3Snapshot(cluster *shared.Cluster, s3Bucket string, s3Folder string, s3Region string) {
	// accessKey := cluster.Config.AwsS3Config.AccessKey
	// secretKey := cluster.Config.AwsS3Config.SecretAccessKey

	// fmt.Println(accessKey)
	// fmt.Println(secretKey)
	shared.LogLevel("info", "adding %s-extra-metadata configmap", cluster.Config.Product)
	// extraMetadataCmd := fmt.Sprintf("kubectl apply -f %s-extra-metadata.yaml", cluster.Config.Product)
	addExtraMetadataConfigMap(cluster)
	// s3Bucket := flags.ExternalFlag.S3Bucket
	// s3Folder := flags.ExternalFlag.S3Folder
	// s3Region := flags.ExternalFlag.S3Region

	fmt.Println()

	os.Getenv("AWS_ACCESS_KEY_ID")
	os.Getenv("AWS_SECRET_ACCESS_KEY")

	// takeS3Snapshot(cluster)

	shared.LogLevel("info", "running cluster reset on server %s\n", cluster.ServerIPs[0])
	// restoreS3Snapshot(cluster, flags)

}

func addExtraMetadataConfigMap(cluster *shared.Cluster) {
	addConfigMapCmd := fmt.Sprintf("sudo kubectl --kubeconfig /etc/rancher/%s/%s.yaml apply -f %s-extra-metadata.yaml", cluster.Config.Product, cluster.Config.Product, cluster.Config.Product)
	shared.RunCommandOnNode(addConfigMapCmd, cluster.ServerIPs[0])
}

// perform snapshot and list snapshot commands -- deploy workloads after snapshot [apply workload]
func takeS3Snapshot(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	// deploy more workloads -- but do not delete them
	// diff command -- comparison of outputs []

}

// this is to be performed after the creation of the fresh VM -- create VM in this function
func restoreS3Snapshot(cluster *shared.Cluster, flags *customflag.FlagConfig) {
	// productLocationCmd, findErr := shared.FindPath(cluster.Config.Product, cluster.ServerIPs[0])
	// Expect(findErr).NotTo(HaveOccurred())
	// resetCmd := fmt.Sprintf("sudo %s server --cluster-reset --etcd-s3 --cluster-reset-restore-path=%s --etcd-s3-bucket=%s --etcd-s3-folder=%s --etcd-s3-region=us-east-2 --etcd-s3-access-key=%s --etcd-s3-secret-key=%s --token=%s", productLocationCmd)
	// resetRes, resetCmdErr := shared.RunCommandOnNode(resetCmd, cluster.ServerIPs[0])
	// Expect(resetCmdErr).NotTo(HaveOccurred())
	// Expect(resetRes).To(ContainSubstring("Managed etcd cluster"))
	// Expect(resetRes).To(ContainSubstring("has been reset"))
}

// make sure the workload you deployed after the snapshot isn't present after the restore snapshot

// func installProduct() {

// }

// func deleteOldNodes() {

// }
