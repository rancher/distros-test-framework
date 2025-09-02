
username = "ShylajaD"
password = "shyAdmin"

# Required variables for legacy infrastructure

# mysql
# external_db           = "mysql"
# external_db_version   = "8.0.41"
# instance_class        = "db.t3.medium"
# db_group_name         = "default.mysql8.0"


#external_db           = "aurora-mysql"
#external_db_version   = "5.7.mysql_aurora.2.11.2"
#instance_class        = "db.t3.medium"
#db_group_name         = "default.aurora-mysql5.7"

external_db             = "postgres"
external_db_version     = "16.3"
db_group_name           = "default.postgres16"
instance_class          = "db.t3.medium"

## mariadb
# external_db           = "mariadb"
# external_db_version   = "10.11.9"
# instance_class        = "db.t3.medium"
# db_group_name         = "default.mariadb10.11"

# create_eip =   true

engine_mode           = "provisioned"
db_username           = "adminuser"
db_password           = "admin1234"

datastore_type          = "etcd"

bastion_ip = ""
bastion_subnets         = "subnet-1ed44d64"
no_of_bastion_nodes = 0
bastion_dns = ""

# enable_public_ip     = false
# enable_ipv6          = false
 
vpc_id             = "vpc-bfccf4d7"

# availability_zone  = "us-east-2b"
 
subnets            = "subnet-ee8cac86"
availability_zone  = "us-east-2a"

# AWS variables
environment        = "local"
region             = "us-east-2"
# region            = "us-west-2"

hosted_zone        = "qa.rancher.space"
#create_lb = true will create a load balancer network + route53 record + target groups
create_lb          =  false
windows_ec2_instance_class = "t3.xlarge"


windows_aws_ami             = "ami-036302f870eb93c1a"

# ec2_instance_class = "t2.small"
# ec2_instance_class = "t3a.xlarge"
ec2_instance_class =  "t3.xlarge"
# ec2_instance_class    = "a1.large"
# ec2_instance_class    = "t3.medium"
# ec2_instance_class    = "g4dn.xlarge"

# ec2_instance_class = "t4g.large"




iam_role           = "RancherK8SUnrestrictedCloudProviderRoleUS"
volume_size        = "40"




# sg_id              = "sg-0e753fd5550206e55"

sg_id              = "sg-08e8243a8cfbea8a0"




# server_flags   = "profile: cis\nselinux: true"
# worker_flags   = "profile: cis\nselinux: true"

# server_flags   = "write-kubeconfig-mode: 644\nselinux: true\nprofile: cis\ncni:\n- multus\n- cilium\npod-security-admission-config-file: /etc/rancher/rke2/custom-psa.yaml"
# server_flags   = "write-kubeconfig-mode: 644\nselinux: true\ncni: canal"
# server_flags   = "write-kubeconfig-mode: 644\nselinux: true\nprofile: cis\npod-security-admission-config-file: /etc/rancher/rke2/custom-psa.yaml"
# server_flags   = "write-kubeconfig-mode: 644\nselinux: true\nprofile: cis\ncni: cilium\npod-security-admission-config-file: /etc/rancher/rke2/custom-psa.yaml"
server_flags = "write-kubeconfig-mode: 644\ncni: calico\nprofile: cis\nselinux: true"

worker_flags = ""
# optional_files  =  "/etc/rancher/rke2/custom-psa.yaml,https://gist.githubusercontent.com/rancher-max/e1c728805b1e5aae8b547b075261bb56/raw/99feb324959d7de9f640d934f098319813202d4a/pod_security_config.yaml"
optional_files = ""


# worker_flags   = "profile: cis\nsecrets-encryption: true\nselinux: true"

# server_flags   = "profile: cis\nsecrets-encryption: true\nselinux: true\ncni:\n- multus\n- canal\n"
#server_flags = "kube-cloud-controller-manager-arg:\n  - allocate-node-cidrs=true\n  - authentication-kubeconfig=/var/lib/rancher/rke2/master/cred/cloud-controller.kubeconfig\n  - authorization-kubeconfig=/var/lib/rancher/rke2/master/cred/cloud-controller.kubeconfig\n  - bind-address=127.0.0.1\n  - cloud-config=/var/lib/rancher/rke2/master/etc/cloud-config.yaml\n  - cloud-provider=rke2\n  - cluster-cidr=10.42.0.0/24\n  - configure-cloud-routes=true\n  - controllers=*,-route,-service\n  - feature-gates=CloudDualStackNodeIPs=true\n  - kubeconfig=/vdsadasdasdasdsadcasller.kuuubs\n  - leader-elect-resource-name=rke2-cloud-controller-manager\n  - node-status-update-frequency=sdadasda\n  - profiling=NO"
#server_flags="kube-cloud-controller-manager-arg:\n  allocate-node-cidrs: \\\"true\\\"\n  authentication-kubeconfig: \\\"/var/lib/rancher/rke2/master/cred/cloud-controller.kubeconfig\\\"\n  authorization-kubeconfig: \\\"/var/lib/rancher/rke2/master/cred/cloud-controller.kubeconfig\\\"\n  bind-address: \\\"127.0.0.1\\\"\n  cloud-config: \\\"/var/lib/rancher/rke2/master/etc/cloud-config.yaml\\\"\n  cloud-provider: \\\"rke2\\\"\n  cluster-cidr: \\\"10.42.0.0/24\\\"\n  configure-cloud-routes: \\\"true\\\"\n  controllers: \\\"*, -route, -service\\\"\n  feature-gates: \\\"CloudDualStackNodeIPs=true\\\"\n  kubeconfig: \\\"/var/lib/rancher/rke2/master/cred/cloud-controller.kubeconfig\\\"\n  leader-elect-resource-name: \\\"rke2-cloud-controller-manager\\\"\n  node-status-update-frequency: \\\"5m0s\\\"\n  profiling: \\\"true\\\""



#version or commit value
rke2_version   = "v1.33.3-rc2+rke2r1"
# install_version = "v1.33.3-rc2+rke2r1"

#valid options: INSTALL_RKE2_VERSION or INSTALL_RKE2_COMMIT
install_mode     = "INSTALL_RKE2_VERSION"
#valid options: 'tar', 'rpm'
install_method   = "rpm"
#valid options: 'latest', 'stable' (default), 'testing'
rke2_channel     = ""

arch             = "amd64"





#ARM
# aws_ami             = "ami-0438c8473dd0c24ce"
# node_os             = "centos8"
# aws_user            = "rocky"



# node_os            = "slemicro"
# aws_ami            = "ami-0534a8d613841ee04"
# aws_user           = "root"


# Custom Vars
# windows_aws_ami      = "ami-05a418fd6eb36fd5b"
#aws_user             = "Administrator"

# node_os            = "opensuse"
# aws_ami             = "ami-0f1f570e75f7b97c5"
# aws_user           = "ec2-user"


#micro
# node_os            = "sles15"
# aws_ami             = "ami-061372818f595fca0"
# aws_user           = "suse"


//mysql server
# aws_ami  =  "ami-03c8fa35fb3dafced"
# aws_user = "ec2-user"
# node_os  = "rhel8"




# aws_ami  =  "ami-0e2cd0a8466d72bb2"
# aws_user = "ec2-user"
# node_os  = "slemicro"


#centos8
#aws_ami  = "ami-0e02efeca352b062c"
#aws_user = "centos"
#node_os  = "centos"


# node_os            = "centos8"
#  aws_ami            = "ami-0ba3fb52acf9675cb"
#  aws_user           = "rocky"


# node_os            = "centos8"
#  aws_ami            = "ami-005726d18930c0d44"
#  aws_user           = "rocky"



## rockie 9.2
# aws_ami    = "ami-0140491b434cb5296"
# node_os            = "centos8"
# aws_user            = "rocky"


#ARM
# node_os            = "ubuntu"
# aws_ami            = "ami-05983a09f7dc1c18f"
# aws_user           = "ubuntu"



# node_os             = "oracle8"
# aws_ami             = "ami-09adbdb528b5a9fe4"
# aws_user            = "ec2-user"


#oracle 9.1
# node_os             = "oracle9"
# aws_ami             = "ami-0d77b6b12ba00534b"
# aws_user            = "cloud-user"

#
# node_os            = "oracle8"
# aws_ami             =  "ami-0287f3009fa848897"
# aws_user            = "ec2-user"

#8.9
# node_os            = "oracle8"
# aws_ami             =  "ami-0287f3009fa848897"
# aws_user            = "ec2-user"




#
# node_os            = "sles15"
# aws_ami            = "ami-01de4781572fa1285"
# aws_user           = "ec2-user"
#
# node_os            = "ubuntu"
# aws_ami            = "ami-085f9c64a9b75eed5"
# aws_user           = "ubuntu"


# node_os            = "sles15"
# aws_ami            = "ami-0e6e78596f3522ace"
# aws_user           = "ec2-user"

#
# node_os            = "sles15"
# aws_ami             = "ami-046cd3113e0c1b581"
# aws_user            = "ec2-user"

# node_os            = "sles15"
# aws_ami            = "ami-0bbc06589f2e4f4f2"
# aws_user           = "ec2-user"



//rhel 9
aws_ami             = "ami-0f6c1051253397fef"
node_os             = "rhel9"
aws_user            = "ec2-user"

# node_os             = "rhel"
# aws_ami = "ami-01315701a80643d63"
# aws_user            = "ec2-user"

# RHEL 8.10 with SQL Server 2022 Standard Edition AMI provided by Amazon.
# aws_ami             = "ami-03c8fa35fb3dafced"
# node_os             = "rhel8"
# aws_user            = "ec2-user"

//volume 100
#node_os            = "rhel8"
#aws_ami            = "ami-08181a4b6882eeb91"
#aws_user           = "ec2-user"



//sle nvivida
# aws_ami             = "ami-0d9e9f236fbe4163f"
# node_os             = "sles15"
# aws_user            = "ec2-user"


# node_os             = "ubuntu"
# aws_ami             = "ami-0283a57753b18025b"
# aws_user            = "ubuntu"





#Add unique resource name to avoid conflicts
resource_name      = "fmoralrke2"




key_name            = "jenkins-rke-validation"
#Run locally use this access_key bellow
#access_key         = "/Users/moral/jenkins-keys/jenkins-rke-validation.pem"

#Run with docker use this access_key bellow
access_key         = "/go/src/github.com/rancher/distros-test-framework/shared/config/.ssh/aws_key.pem"

##################  Please be careful with the following variables and configuration  ##################
## split_roles must be always true if you want to split the roles
## nodes must be always filled with the total number of nodes or 0
# role_order is the order in which the roles will be assigned to the nodes
# Numbers 1-6 correspond to: all-roles (1), etcd-only (2), etcd-cp (3), etcd-worker (4), cp-only (5), cp-worker (6)
no_of_windows_worker_nodes= 0

split_roles        = false
no_of_server_nodes = 1
no_of_worker_nodes = 0
etcd_only_nodes    = 0
etcd_cp_nodes      = 0
etcd_worker_nodes  = 0
cp_only_nodes      = 0
cp_worker_nodes    = 0
role_order         = "1,2,3,4,5,6"



## CERT ROTATE
# split_roles        = true
# no_of_server_nodes = 0
# no_of_worker_nodes = 1
# etcd_only_nodes    = 3
# etcd_cp_nodes      = 0
# etcd_worker_nodes  = 0
# cp_only_nodes      = 2
# cp_worker_nodes    = 0
# # Numbers 1-6 correspond to: all-roles (1), etcd-only (2), etcd-cp (3), etcd-worker (4), cp-only (5), cp-worker (6).
# role_order         = "2,5"




// secrets
# split_roles        = true
# no_of_server_nodes = 0
# no_of_worker_nodes = 1
# etcd_only_nodes    = 3
# etcd_cp_nodes      = 0
# etcd_worker_nodes  = 0
# cp_only_nodes      = 2
# cp_worker_nodes    = 0
# role_order         = "2,5"


#sle micro: ami-021b1f638e534e276
#coreos: ami-0b2784ccfbe9dc5b5
