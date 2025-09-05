package testcase

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/config"

	"github.com/rancher/distros-test-framework/internal/resources"

	"github.com/rancher/distros-test-framework/internal/pkg/aws"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/k8s"

	. "github.com/onsi/gomega"
)

func TestClusterRestore(cluster *resources.Cluster, awsClient *aws.Client, cfg *config.Env, flags *customflag.FlagConfig) {
	// TODO: Restore product.FetchToken function
	clusterToken := "placeholder-token"
	// clusterToken, clusterTokenErr := product.FetchToken(cluster.Config.Product, cluster.ServerIPs[0])
	// Expect(clusterTokenErr).NotTo(HaveOccurred())

	onDemandPath := s3Snapshot(cluster, awsClient, flags)
	stopInstances(cluster, awsClient)

	_, newServerIP := newInstance(awsClient)

	// TODO: Restore product.InstallProduct function
	// err := product.InstallProduct(cluster, newServerIP, cfg.InstallVersion)
	// Expect(err).NotTo(HaveOccurred())

	restoreS3Snapshot(cluster, onDemandPath, clusterToken, newServerIP, flags)
	enableErr := resources.EnableAndStartService(cluster, newServerIP, "server")
	Expect(enableErr).NotTo(HaveOccurred())

	// TODO: Restore clusterPkg.NewLocalKubeconfigFile function
	// kubeConfigErr := clusterPkg.NewLocalKubeconfigFile(newServerIP, serverName, cluster.Config.Product,
	//	"/tmp/"+serverName+"_kubeconfig")
	// Expect(kubeConfigErr).NotTo(HaveOccurred())

	// create k8s client now because it depends on newly created kubeconfig file.
	k8sClient, k8sErr := k8s.AddClient()
	Expect(k8sErr).NotTo(HaveOccurred())

	deleteOldNodes(cluster)
	postValidationRestore(k8sClient)
	updateClusterIPs(cluster, newServerIP)
	deleteS3Snapshot(awsClient, flags, onDemandPath)
}

// s3Snapshot deploys extra metadata to take a snapshot of the cluster to s3 and returns the path of the snapshot.
func s3Snapshot(cluster *resources.Cluster, awsClient *aws.Client, flags *customflag.FlagConfig) string {
	workloadErr := resources.ManageWorkload("apply", "extra-metadata.yaml")
	Expect(workloadErr).NotTo(HaveOccurred(), "configmap failed to create")

	takeS3Snapshot(cluster, flags)

	onDemandPath, onDemandPathErr := resources.RunCommandOnNode(fmt.Sprintf("sudo ls /var/lib/rancher/%s/server/db/snapshots",
		cluster.Config.Product), cluster.ServerIPs[0])
	Expect(onDemandPathErr).NotTo(HaveOccurred())

	validateS3snapshot(awsClient, flags, onDemandPath)

	return onDemandPath
}

func takeS3Snapshot(cluster *resources.Cluster, flags *customflag.FlagConfig) {
	productLocationCmd, findErr := resources.FindPath(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(findErr).NotTo(HaveOccurred())

	takeSnapshotCmd := fmt.Sprintf("sudo %s etcd-snapshot save --s3 --s3-bucket=%s "+
		"--s3-folder=%s --s3-region=%s --s3-access-key=%s --s3-secret-key=%s",
		productLocationCmd, flags.S3Flags.Bucket, flags.S3Flags.Folder, cluster.Aws.Region,
		cluster.Aws.AccessKeyID, cluster.Aws.SecretAccessKey)
	snapshotResponse, takeSnapshotErr := resources.RunCommandOnNode(takeSnapshotCmd, cluster.ServerIPs[0])
	Expect(takeSnapshotErr).NotTo(HaveOccurred())

	resources.LogLevel("info", "snapshot taken in s3: %s", snapshotResponse)
}

func validateS3snapshot(awsClient *aws.Client, flags *customflag.FlagConfig, onDemandPath string) {
	s3List, s3ListErr := awsClient.GetObjects(flags.S3Flags.Bucket)
	Expect(s3ListErr).NotTo(HaveOccurred())
	for _, listObject := range s3List {
		if strings.Contains(*listObject.Key, onDemandPath) {
			resources.LogLevel("info", "snapshot found: %s", onDemandPath)
			break
		}
	}

	resources.LogLevel("info", "successfully validated snapshot save in s3: %s/%s", flags.S3Flags.Bucket, onDemandPath)
}

func stopInstances(cluster *resources.Cluster, ec2 *aws.Client) {
	var instancesIPs []string

	instancesIPs = append(instancesIPs, cluster.ServerIPs...)
	instancesIPs = append(instancesIPs, cluster.AgentIPs...)

	for _, ip := range instancesIPs {
		id, idsErr := ec2.GetInstanceIDByIP(ip)
		Expect(idsErr).NotTo(HaveOccurred())
		err := ec2.StopInstance(id)
		Expect(err).NotTo(HaveOccurred())
	}
}

func newInstance(awsClient *aws.Client) (newServerName, newExternalIP string) {
	resourceName := os.Getenv("resource_name")
	var serverName []string
	serverName = append(serverName, resourceName+"-server-fresh")

	externalServerIP, _, _, createErr := awsClient.CreateInstances(serverName...)
	Expect(createErr).NotTo(HaveOccurred(), createErr)

	return serverName[0], externalServerIP[0]
}

func restoreS3Snapshot(
	cluster *resources.Cluster,
	onDemandPath,
	token string,
	newClusterIP string,
	flags *customflag.FlagConfig,
) {
	var (
		restoreCmdRes string
		restoreCmdErr error
	)
	productLocationCmd, findErr := resources.FindPath(cluster.Config.Product, newClusterIP)
	Expect(findErr).NotTo(HaveOccurred())

	restoreCmd := fmt.Sprintf("sudo %s server --cluster-reset --etcd-s3 --cluster-reset-restore-path=%s"+
		" --etcd-s3-bucket=%s --etcd-s3-folder=%s --etcd-s3-region=%s --etcd-s3-access-key=%s"+
		" --etcd-s3-secret-key=%s --token=%s",
		productLocationCmd,
		onDemandPath,
		flags.S3Flags.Bucket,
		flags.S3Flags.Folder,
		cluster.Aws.Region,
		cluster.Aws.AccessKeyID,
		cluster.Aws.SecretAccessKey,
		token)

	switch cluster.Config.Product {
	case "k3s":
		restoreCmdRes, restoreCmdErr = resources.RunCommandOnNode(restoreCmd, newClusterIP)
		Expect(restoreCmdErr).NotTo(HaveOccurred())
		Expect(restoreCmdRes).To(ContainSubstring("Managed etcd cluster"))
		Expect(restoreCmdRes).To(ContainSubstring("has been reset"))
	case "rke2":
		_, restoreCmdErr = resources.RunCommandOnNode(restoreCmd, newClusterIP)
		Expect(restoreCmdErr).To(HaveOccurred())
		Expect(restoreCmdErr.Error()).To(Not(BeNil()))
		Expect(restoreCmdErr.Error()).To(ContainSubstring("Managed etcd cluster"))
		Expect(restoreCmdErr.Error()).To(ContainSubstring("has been reset"))
	default:
		Expect(fmt.Errorf("product not supported: %s", cluster.Config.Product)).NotTo(HaveOccurred())
	}
}

func deleteOldNodes(cluster *resources.Cluster) {
	resources.LogLevel("debug", "deleting old nodes")

	var oldNodeIPs []string
	oldNodeIPs = append(oldNodeIPs, cluster.ServerIPs...)
	oldNodeIPs = append(oldNodeIPs, cluster.AgentIPs...)
	// TODO: Restore clusterPkg.DeleteNode function
	// for _, ip := range oldNodeIPs {
	//	err := clusterPkg.DeleteNode(ip)
	//	Expect(err).NotTo(HaveOccurred())
	// }
}

// postValidationRestore validate overall cluster health after restore, one node (new one) should be in Ready state.
func postValidationRestore(k8sClient *k8s.Client) {
	ok, err := k8sClient.CheckClusterHealth(1)
	Expect(err).NotTo(HaveOccurred())
	Expect(ok).To(BeTrue())

	resources.PrintClusterState()
}

func updateClusterIPs(cluster *resources.Cluster, newServerIP string) {
	resources.LogLevel("info", "Updating cluster IPs with new server IP: %s", newServerIP)

	cluster.ServerIPs = []string{newServerIP}
	cluster.NumServers = 1
	cluster.NumAgents = 0
	cluster.AgentIPs = []string{}
}

func deleteS3Snapshot(awsClient *aws.Client, flags *customflag.FlagConfig, name string) {
	resources.LogLevel("info", "cleaning s3 snapshots")

	err := awsClient.DeleteS3Object(flags.S3Flags.Bucket, flags.S3Flags.Folder, name)
	if err != nil {
		resources.LogLevel("error", "error deleting object: %v", err)
	}
}
