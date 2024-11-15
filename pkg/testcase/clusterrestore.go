package testcase

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/pkg/assert"
	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/gomega"
)

var awsConfig shared.AwsConfig

func TestClusterRestore(cluster *shared.Cluster, applyWorkload bool, flags *customflag.FlagConfig) {
	setConfigs()

	product, version, err := shared.Product()
	Expect(err).NotTo(HaveOccurred())

	version = cleanVersionData(product, version)

	if applyWorkload {
		workloadErr := shared.ManageWorkload("apply", product+"-extra-metadata.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "configmap failed to create")
	}

	takeS3Snapshot(cluster, flags)
	shared.LogLevel("info", "snapshot taken in s3")

	onDemandPath, onDemandPathErr := shared.RunCommandOnNode(fmt.Sprintf("sudo ls /var/lib/rancher/%s/server/db/snapshots",
		product), cluster.ServerIPs[0])
	Expect(onDemandPathErr).NotTo(HaveOccurred())

	validateS3snapshots(cluster, flags, onDemandPath)
	shared.LogLevel("info", "successfully validated s3 snapshot save in s3")

	clusterToken, clusterTokenErr := shared.FetchToken(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(clusterTokenErr).NotTo(HaveOccurred())

	resourceName := os.Getenv("resource_name")
	ec2, err := aws.AddClient(cluster)
	Expect(err).NotTo(HaveOccurred(), "error adding aws nodes: %s", err)

	stopInstances(cluster, ec2)

	var serverName []string
	serverName = append(serverName, fmt.Sprintf("%s-server-fresh", resourceName))

	externalServerIP, _, _, createErr := ec2.CreateInstances(serverName...)
	Expect(createErr).NotTo(HaveOccurred(), createErr)
	newServerIP := externalServerIP[0]

	installProduct(cluster, newServerIP, version)
	shared.LogLevel("info", "%s successfully installed on server: %s", product, newServerIP)

	restoreS3Snapshot(cluster, onDemandPath, clusterToken, newServerIP, flags)

	enableAndStartService(cluster, newServerIP)
	shared.LogLevel("info", "%s service successfully enabled", product)

	_, copyCmdErr := shared.RunCommandHost("cp /tmp/%s_kubeconfig /tmp/%s_kubeconfig", resourceName, serverName[0])
	Expect(copyCmdErr).NotTo(HaveOccurred())
	_, kubeConfigErr := shared.UpdateKubeConfig(newServerIP, serverName[0], product)
	Expect(kubeConfigErr).NotTo(HaveOccurred())

	postValidationRestore(cluster, newServerIP)
	shared.LogLevel("info", "%s server successfully validated post restore", product)
}

func cleanVersionData(product, version string) string {
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
) {
	productLocationCmd, findErr := shared.FindPath(cluster.Config.Product, cluster.ServerIPs[0])
	Expect(findErr).NotTo(HaveOccurred())

	takeSnapshotCmd := fmt.Sprintf("sudo %s etcd-snapshot save --s3 --s3-bucket=%s "+
		"--s3-folder=%s --s3-region=%s --s3-access-key=%s --s3-secret-key=%s",
		productLocationCmd, flags.S3Flags.Bucket, flags.S3Flags.Folder, cluster.Aws.Region,
		awsConfig.AccessKeyID, awsConfig.SecretAccessKey)

	_, takeSnapshotErr := shared.RunCommandOnNode(takeSnapshotCmd, cluster.ServerIPs[0])
	Expect(takeSnapshotErr).NotTo(HaveOccurred())

	TestDNSAccess(true, false)
}

func validateS3snapshots(cluster *shared.Cluster, flags *customflag.FlagConfig, onDemandPath string) {
	s3, err := aws.AddClient(cluster)
	Expect(err).NotTo(HaveOccurred(), "error creating s3 client: %s", err)

	s3List, s3ListErr := s3.GetObjects(flags)
	Expect(s3ListErr).NotTo(HaveOccurred())
	for _, listObject := range s3List {
		if strings.Contains(*listObject.Key, onDemandPath) {
			shared.LogLevel("info", "snapshot found: %s", onDemandPath)
			break
		}
	}
}

func stopInstances(cluster *shared.Cluster, ec2 *aws.Client) {
	var instancesIPs []string

	instancesIPs = append(instancesIPs, cluster.ServerIPs...)
	instancesIPs = append(instancesIPs, cluster.AgentIPs...)

	for _, ip := range instancesIPs {
		id, idsErr := ec2.GetInstanceIDByIP(ip)
		Expect(idsErr).NotTo(HaveOccurred())
		//
		err := ec2.StopInstance(id)
		if err != nil {
			return
		}
	}
}

func setConfigFile(product, newClusterIP string) {
	createConfigFileCmd := fmt.Sprintf("sudo  bash -c 'cat <<EOF>/etc/rancher/%s/config.yaml\n"+
		"write-kubeconfig-mode: 644\n"+
		"node-external-ip: %s\n"+
		"cluster-init: true\n"+
		"EOF'", product, newClusterIP)

	path := fmt.Sprintf("/etc/rancher/%s/", product)
	cmd := fmt.Sprintf("sudo  mkdir -p %s && %s", path, createConfigFileCmd)

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
		installCmd += fmt.Sprintf("https://get.%s.io/ | sudo INSTALL_%s_VERSION=%s  INSTALL_%s_SKIP_ENABLE=true sh -",
			cluster.Config.Product, strings.ToUpper(cluster.Config.Product), version,
			strings.ToUpper(cluster.Config.Product))
	} else {
		installCmd += fmt.Sprintf("https://get.%s.io | sudo INSTALL_%s_VERSION=%s sh -",
			cluster.Config.Product, strings.ToUpper(cluster.Config.Product), version)
	}

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

	shared.LogLevel("info", "Starting service, waiting for service to complete background processes.")
	Expect(startServiceCmdErr).NotTo(HaveOccurred())

	time.Sleep(120 * time.Second)

	statusServiceCmdRes, statusServiceCmdErr := shared.ManageService(cluster.Config.Product, "status", "server",
		[]string{newClusterIP})
	Expect(statusServiceCmdErr).NotTo(HaveOccurred())
	fmt.Println("STATUS SERVICE OUT: ", statusServiceCmdRes)
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

	productLocationCmd, findErr := shared.FindPath(cluster.Config.Product, newClusterIP)
	Expect(findErr).NotTo(HaveOccurred())

	restoreCmd := fmt.Sprintf("sudo %s server --cluster-reset --etcd-s3 --cluster-reset-restore-path=%s"+
		" --etcd-s3-bucket=%s --etcd-s3-folder=%s --etcd-s3-region=%s --etcd-s3-access-key=%s"+
		" --etcd-s3-secret-key=%s --token=%s",
		productLocationCmd,
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

	exportKubeConfigCmd := fmt.Sprintf("export KUBECONFIG=/etc/rancher/%s/%s.yaml",
		cluster.Config.Product, cluster.Config.Product)

	var pathCmd string
	var kubectlCmd string
	var kubectlCmdErr error
	if cluster.Config.Product == "rke2" {
		pathCmd = fmt.Sprintf("PATH=$PATH:/var/lib/rancher/%s/bin", cluster.Config.Product)
		kubectlCmd = fmt.Sprintf("/var/lib/rancher/%s/bin/kubectl", cluster.Config.Product)
		kubectlCmd = exportKubeConfigCmd + " && " + pathCmd + " && " + kubectlCmd
		fmt.Println("KUBECTL CMD: ", kubectlCmd)
	} else {
		pathCmd = "PATH=$PATH:/usr/local/bin"
		kubectlCmd, kubectlCmdErr = shared.RunCommandOnNode("which kubectl", newServerIP)
		Expect(kubectlCmdErr).NotTo(HaveOccurred())
		kubectlCmd = exportKubeConfigCmd + " && " + pathCmd + " && " + kubectlCmd
	}

	getNodesPodsCmd := kubectlCmd + fmt.Sprintf(" get nodes,pods -A -o wide %s", kubeconfigFlagRemote)
	_, nodesPodsErr := shared.RunCommandOnNode(getNodesPodsCmd, newServerIP)
	Expect(nodesPodsErr).NotTo(HaveOccurred())

	shared.PrintClusterState()
	time.Sleep(20 * time.Second)

	var oldNodeIPs []string
	oldNodeIPs = append(oldNodeIPs, cluster.ServerIPs...)
	oldNodeIPs = append(oldNodeIPs, cluster.AgentIPs...)
	for _, ip := range oldNodeIPs {
		err := shared.DeleteNode(ip)
		if err != nil {
			shared.LogLevel("error", "error deleting nodes")
		}
	}

	shared.LogLevel("info", "deleting old nodes")
	time.Sleep(240 * time.Second)

	testIngressPostRestore(newServerIP, true, true, kubectlCmd)
	shared.LogLevel("info", "ingress successfully validated post cluster restore")

	testClusterIPPostRestore(newServerIP, true, true, kubectlCmd)
	shared.LogLevel("info", "clusterIP successfully validated post cluster restore")

	testNodePortPostRestore(newServerIP, false, true, kubectlCmd)
	shared.LogLevel("info", "nodeport successfully validated post cluster restore")

	testDNSAccessPostRestore(newServerIP, kubectlCmd)
	shared.LogLevel("info", "dns successfully validated post cluster restore")

	testValidateNodesPostRestore(newServerIP)
	shared.LogLevel("info", "nodes post restore have been validated")

	testValidatePodsPostRestore()
	shared.LogLevel("info", "pods post restore have been validated")
}

func testIngressPostRestore(newServerIP string, applyWorkload, deleteWorkload bool, kubectlCmd string) {
	if applyWorkload {
		workloadErr := shared.ManageWorkload("apply", "ingress.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "ingress manifest not deployed")
	}

	ingressErr := assert.ValidateOnNode(newServerIP, kubectlCmd+" get pods -n test-ingress -l k8s-app=nginx-app-ingress"+
		" --field-selector=status.phase=Running", "Running")
	Expect(ingressErr).NotTo(HaveOccurred())

	if deleteWorkload {
		workloadErr := shared.ManageWorkload("delete", "ingress.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Ingress manifest not deleted")
	}
}

func testNodePortPostRestore(newServerIP string, applyWorkload, deleteWorkload bool, kubectlCmd string) {
	if applyWorkload {
		workloadErr := shared.ManageWorkload("apply", "nodeport.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "NodePort manifest not deployed")
	}

	nodePortErr := assert.ValidateOnNode(newServerIP, kubectlCmd+" get pods -n test-nodeport -l k8s-app=nginx-app-nodeport "+
		"--field-selector=status.phase=Running", "Running")
	Expect(nodePortErr).NotTo(HaveOccurred())

	if deleteWorkload {
		workloadErr := shared.ManageWorkload("delete", "nodeport.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "NodePort manifest not deleted")
	}
}

func testClusterIPPostRestore(newServerIP string, applyWorkload, deleteWorkload bool, kubectlCmd string) {
	if applyWorkload {
		workloadErr := shared.ManageWorkload("apply", "clusterip.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Cluster IP manifest not deployed")
	}

	clusterIPErr := assert.ValidateOnNode(newServerIP, kubectlCmd+" get pods -n test-clusterip -l"+
		" k8s-app=nginx-app-clusterip "+"--field-selector=status.phase=Running", "Running")
	Expect(clusterIPErr).NotTo(HaveOccurred())

	if deleteWorkload {
		workloadErr := shared.ManageWorkload("delete", "clusterip.yaml")
		Expect(workloadErr).NotTo(HaveOccurred(), "Cluster IP manifest not deleted")
	}
}

func testDNSAccessPostRestore(newServerIP, kubectlCmd string) {
	dnsErr := assert.ValidateOnNode(newServerIP, kubectlCmd+" get pods -n dnsutils dnsutils")
	Expect(dnsErr).To(HaveOccurred())
}

func testValidateNodesPostRestore(newServerIP string) {
	res, err := shared.GetNodes(true)
	Expect(err).NotTo(HaveOccurred())
	fmt.Println("Nodes: ", res)
	Expect(res).NotTo(BeEmpty())
	for _, node := range res {
		if strings.Contains(node.ExternalIP, newServerIP) && strings.Contains(node.Status, "Ready") {
			shared.LogLevel("info", "%s node is Ready", newServerIP)
		} else {
			shared.LogLevel("error", "%s node is NotReady", newServerIP)
		}
	}
}

func testValidatePodsPostRestore() {
	res, err := shared.GetPods(true)
	Expect(err).NotTo(HaveOccurred())
	fmt.Println("Pods: ", res)
	Expect(res).NotTo(BeEmpty())
	serverFlags := os.Getenv("server_flags")
	workerFlags := os.Getenv("worker_flags")
	for _, pod := range res {
		if strings.Contains(serverFlags, "calico") && strings.Contains(pod.NameSpace, "calico-system") {
			if strings.Contains(pod.Status, "Completed") || strings.Contains(pod.Status, "Running") {
				shared.LogLevel("info", "calico-system pods have been successfully validated")
			} else {
				shared.LogLevel("error", "unable to validate calico-system pods")
			}
		}
		if strings.Contains(serverFlags, "calico") && strings.Contains(workerFlags, "multus") &&
			strings.Contains(pod.NameSpace, "tigera-operator") {
			if strings.Contains(pod.Status, "Completed") || strings.Contains(pod.Status, "Running") {
				shared.LogLevel("info", "tigera-operator pods have been successfully validated")
			} else {
				shared.LogLevel("error", "unable to validate tigera-operator pods")
			}
		}
		if strings.Contains(pod.NameSpace, "kube-system") {
			if strings.Contains(pod.Status, "Completed") || strings.Contains(pod.Status, "Running") {
				shared.LogLevel("info", "pods have been successfully validated")
			} else {
				shared.LogLevel("error", "unable to validate pods")
			}
		} else {
			shared.LogLevel("error", "failed to validate nodes")
		}
	}
}
