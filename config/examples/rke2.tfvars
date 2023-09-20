## RKE2 variables -- Fill these in with your desired values.

rke2_version="v1.27.4+rke2r1"
rke2_channel="testing"
server_flags   = "profile: cis\n"
worker_flags   = "profile: cis\n"
# For hardened on v1.23 or v1.24, set the server and worker flags to be: "selinux: true\nprofile: cis-1.6". 
# For v1.25+: "selinux: true\nprofile: cis" Note: "profile: cis-1.23" is getting deprecated.  
# If using optional PSA, make sure to include that in the server_flags to: pod-security-admission-config-file: /etc/rancher/rke2/custom-psa.yaml

resource_name  = "<prefix_name_for_your_resources>"
key_name       = "jenkins-rke-validation"
access_key     = "/go/src/github.com/rancher/distros-test-framework/config/.ssh/aws_key.pem"
password       = "<enter_your_password>"

create_lb      = false
arch           = "amd64"
# "/etc/rancher/rke2/custom-psa.yaml,https://gist.githubusercontent.com/rancher-max/e1c728805b1e5aae8b547b075261bb56/raw/99feb324959d7de9f640d934f098319813202d4a/pod_security_config.yaml"
optional_files = ""
# INSTALL_RKE2_VERSION or INSTALL_RKE2_COMMIT
install_mode   = "INSTALL_RKE2_VERSION"
# leave blank or choose 'tar' or 'rpm'; For selinux testing, set to 'rpm' mode of install
install_method = ""

## Windows agent variables
#server_flags   = "cni: calico\n"
windows_ec2_instance_class  = "t3.xlarge"
windows_aws_ami             = "<ami-id>"
no_of_windows_worker_nodes  = 0

## Custom Vars
node_os            = "sles15"
aws_ami            = "<ami-id>"
aws_user           = "ec2-user"
# This is also known as an "all-roles" node
no_of_server_nodes = 3
no_of_worker_nodes = 1
split_roles        = false
etcd_only_nodes    = 0
etcd_cp_nodes      = 0
etcd_worker_nodes  = 0
cp_only_nodes      = 0
cp_worker_nodes    = 0
# Numbers 1-6 correspond to: all-roles (1), etcd-only (2), etcd-cp (3), etcd-worker (4), cp-only (5), cp-worker (6).
role_order         = "1,2,3,4,5,6"

## Rhel7 variables
username = "ShylajaD"

## AWS variables
region             = "us-east-2"
hosted_zone        = "qa.rancher.space"
ec2_instance_class = "t3a.medium"
vpc_id             = "<vpc-id>"
subnets            = "<subnet-id>"
availability_zone  = "us-east-2a"
sg_id              = "<sg-id>"
iam_role           = "<iam_role>"
volume_size        = "20"
