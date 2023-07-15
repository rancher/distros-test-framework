
username = "ShylajaD"
password = "shyAdmin"

# AWS variables
region             = "us-east-2"
hosted_zone        = "qa.rancher.space"
#create_lb = true will create a load balancer network + route53 record + target groups
create_lb          = false
windows_ec2_instance_class = "t3.xlarge"
ec2_instance_class = "t3.medium"
vpc_id             = "vpc-bfccf4d7"
subnets            = "subnet-ee8cac86"
availability_zone  = "us-east-2a"
sg_id              = "sg-0e753fd5550206e55"
iam_role           = "RancherK8SUnrestrictedCloudProviderRoleUS"
volume_size        = "20"



#For v1.25+: "selinux: true\nprofile: cis-1.23"
#server_flags   = "selinux: true\nprofile: cis-1.23"
#worker_flags   = "selinux: true\nprofile: cis-1.23"
server_flags = ""
worker_flags = ""
#version or commit value
rke2_version   = "33caf61cf1fc71cca522eaeac1d3541b5f3c417c"

#valid options: INSTALL_RKE2_VERSION or INSTALL_RKE2_COMMIT
install_mode   = "INSTALL_RKE2_COMMIT"
#valid options: 'tar', 'rpm'
install_method = ""
#valid options: 'latest', 'stable' (default), 'testing'
rke2_channel   = "stable"


# Custom Vars

windows_aws_ami = "ami-05a418fd6eb36fd5b"

#node_os            = "rhel"
#aws_ami            = "ami-015d9f9ba68b67486"

#node_os            = "sles15"
#aws_ami             = "ami-046cd3113e0c1b581"

#node_os            = "oracle8"
#aws_ami            = "ami-054a49e0c0c7fce5c"

node_os             = "ubuntu"
aws_ami             = "ami-097a2df4ac947655f"

#node_os             = "rhel"
#aws_ami             = "ami-08079ea9aa44b5de6"

#aws_user            = "ec2-user"
aws_user             = "ubuntu"
#aws_user            = "cloud-user"



ssh_key             = "jenkins-rke-validation"
#Run locally use this access_key bellow
access_key         = "/Users/moral/jenkins-keys/jenkins-rke-validation.pem"

#Run with docker use this access_key bellow
#access_key         = "/go/src/github.com/rancher/rke2/tests/acceptance/modules/config/.ssh/aws_key.pem"

##################  Please be careful with the following variables and configuration  ##################
## split_roles must be always true if you want to split the roles
## nodes must be always filled with the total number of nodes or 0
# role_order is the order in which the roles will be assigned to the nodes
# Numbers 1-6 correspond to: all-roles (1), etcd-only (2), etcd-cp (3), etcd-worker (4), cp-only (5), cp-worker (6)
no_of_windows_worker_nodes= 0
split_roles        = false
no_of_server_nodes = 1
no_of_worker_nodes = 1
etcd_only_nodes    = 0
etcd_cp_nodes      = 0
etcd_worker_nodes  = 0
cp_only_nodes      = 0
cp_worker_nodes    = 0
role_order         = "1,2,3,4,5,6"

#Add unique resource name to avoid conflicts
resource_name      = "franmoralrke2local"

optional_files     = ""
# "/etc/rancher/rke2/custom-psa.yaml,https://gist.githubusercontent.com/rancher-max/e1c728805b1e5aae8b547b075261bb56/raw/99feb324959d7de9f640d934f098319813202d4a/pod_security_config.yaml"

