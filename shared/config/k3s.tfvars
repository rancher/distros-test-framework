username = "ShylajaD"
password = "shyAdmin"

create_lb             = false
create_eip            = false




# node_os            = "sles15"
# aws_ami            = "ami-0e6e78596f3522ace"
# aws_user           = "ec2-user"


# node_os            = "centos8"
# aws_ami            = "ami-005726d18930c0d44"
# aws_user           = "rocky"

# node_os            = "rhel8"
#  aws_ami            = "ami-0de11975b572ed425"
# aws_user           = "ec2-user"


#
# node_os            = "slemicro"
# aws_ami            = "ami-0534a8d613841ee04"
# aws_user           = "root"



#ARM
# aws_ami             = "ami-0438c8473dd0c24ce"
# node_os             = "centos8"
# aws_user            = "rocky"


## rockie 9.2
# aws_ami            = "ami-0140491b434cb5296"
# node_os            = "centos9"
# aws_user            = "rocky"


//rhel 9
# aws_ami             = "ami-0f6c1051253397fef"
# node_os             = "rhel9"
# aws_user            = "ec2-user"






#8.7
# aws_ami            = "ami-05ab2eb74c93eb441"
# node_os            = "centos9"
# aws_user            = "rocky"


## oracle 9.2
# node_os             = "oracle8"
# aws_ami             = "ami-02063ef277481f6df"
# aws_user            = "cloud-user"



#ARM ubuntu
# aws_ami             = "ami-05983a09f7dc1c18f"
# node_os             = "ubuntu"
# aws_user            = "ubuntu"



#node_os            = "oracle8"
#aws_ami            = "ami-054a49e0c0c7fce5c"

#8.9
# node_os            = "oracle8"
# aws_ami             =  "ami-0287f3009fa848897"
# aws_user            = "ec2-user"



# node_os            = "sles15"
# aws_ami            = "ami-046cd3113e0c1b581"
# aws_user           = "ec2-user"

#aws_user            = "suse"
#aws_ami =   "ami-061372818f595fca0"
#node_os            = "sles15"
#ami-0283a57753b18025b

#
# aws_ami  =  "ami-084cf4d332a139314"
# aws_user = "ec2-user"
# node_os  = "rhel8"


# node_os            = "rhel8"
# aws_ami            = "ami-0fb29513fe88cc9dc"
# aws_user           = "ec2-user"


// airgap
# node_os            = "sles15"
# aws_ami            = "ami-003687d23776452a2"
# aws_user           = "ec2-user"

node_os            = "ubuntu"
aws_ami            = "ami-085f9c64a9b75eed5" # Ubuntu 24.04 LTS
aws_user           = "ubuntu"




# aws_ami  = "ami-0613127c0b3b6972b"
# aws_user = "ec2-user"
# node_os  = "sles15"

#arm
# aws_ami             = "ami-0438c8473dd0c24ce"
# node_os             = "centos8"
# aws_user            = "rocky"

# node_os            = "sles15"
# aws_ami            = "ami-0e6e78596f3522ace"
# aws_user           = "ec2-user"

# node_os            = "rhel8"
# aws_ami            =  "ami-0c63004eca0417ff0"
# aws_user           = "ec2-user"

# node_os             = "ubuntu"
# aws_ami             = "ami-0283a57753b18025b"
# aws_user            = "ubuntu"
# #

#
# node_os            = "rhel8"
# aws_ami            = "ami-05ab2eb74c93eb441"
# aws_user           = "rocky"

# node_os            = "opensuse"
# aws_ami             = "ami-022f87d3cf4f1e67c"
# aws_user           = "ec2-user"


// ubuntu docker
# node_os            = "ubuntu"
# aws_ami            = "ami-09457fad1d2c34c31"
# aws_user           = "ubuntu"

# node_os             = "rhel8"
# ####aws_ami             = "ami-0defbb5087b2b63c1"
# aws_ami             = "ami-082bf7cc12db545b9"
# aws_user            = "ec2-user"

//volume 100
#node_os            = "rhel8"
#aws_ami            = "ami-08181a4b6882eeb91"
#aws_user           = "ec2-user"


##############  external db variables  #################

# datastore_type "external" = using external db | datastore_type "etcd" = using etcd
datastore_type          = "etcd"


external_db             = "postgres"
external_db_version     = "16.3"
db_group_name           = "default.postgres16"
instance_class          = "db.t3.medium"

#aurora-mysql
#external_db           = "aurora-mysql"
#external_db_version   = "5.7.mysql_aurora.2.11.2"
#instance_class        = "db.t3.medium"
#db_group_name         = "default.aurora-mysql5.7"


# mysql
# external_db           = "mysql"
# external_db_version   = "8.0.41"
# instance_class        = "db.t3.medium"
# db_group_name         = "default.mysql8.0"

## mariadb
#external_db           = "mariadb"
#external_db_version   = "10.11.9"
#instance_class        = "db.t3.medium"
#db_group_name         = "default.mariadb10.6"

engine_mode           = "provisioned"
db_username           = "adminuser"
db_password           = "admin1234"

# AWS variables
#ec2_instance_class = "m5.xlarge"
#arm
#ec2_instance_class    = "t4g.small"
# ec2_instance_class    = "a1.large"

# ec2_instance_class    = "t3a.medium"
ec2_instance_class    = "t3.xlarge"
vpc_id                = "vpc-bfccf4d7"

bastion_dns = ""

# availability_zone   = "us-east-2b"
# subnets            = "subnet-1ed44d64"

availability_zone   = "us-east-2a"
subnets             = "subnet-ee8cac86"

# no_of_bastion_nodes  = 1
# enable_public_ip    = false
# enable_ipv6         = false
# bastion_subnets     = "subnet-0377a1ca391d51cae"
bastion_subnets         = "subnet-1ed44d64"

# sg_id                 = "sg-0e753fd5550206e55"
sg_id              = "sg-08e8243a8cfbea8a0"


volume_size          = "60"

region                = "us-east-2"
qa_space              = "qa.rancher.space"


#iam_role = "RancherK8SUnrestrictedCloudProviderRoleUS"



# server_flags   = "docker: true\n"
# worker_flags   = "docker: true\n"
server_flags   =  ""
# server_flags   = "protect-kernel-defaults: true\nselinux: true"
# worker_flags   = "protect-kernel-defaults: true\nselinux: true"
worker_flags   = ""




# install_version = "v1.33.3-rc1+k3s1"
k3s_version           = "v1.33.3-rc1+k3s1"

k3s_channel           = "testing"

install_mode          = "INSTALL_K3S_VERSION"

environment           = "local"
key_name              = "jenkins-elliptic-validation"

#Run locally use this access_key bellow
#access_key            = "/Users/moral/jenkins-keys/jenkins-elliptic-validation.pem"

#Run with docker use this access_key bellow
access_key            = "/go/src/github.com/rancher/distros-test-framework/shared/config/.ssh/aws_key.pem"




##################  Please be careful with the following variables and configuration  ##################
## split_roles must be always true if you want to split the roles
## nodes must be always filled with the total number of nodes or 0
# role_order is the order in which the roles will be assigned to the nodes
# Numbers 1-6 correspond to: all-roles (1), etcd-only (2), etcd-cp (3), etcd-worker (4), cp-only (5), cp-worker (6)
split_roles        = false
no_of_server_nodes = 1
no_of_worker_nodes = 0
etcd_only_nodes    = 0
etcd_cp_nodes      = 0
etcd_worker_nodes  = 0
cp_only_nodes      = 0
cp_worker_nodes    = 0
role_order         = "1,2,3,4,5,6"



// cert-rotate
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





arch               = "amd64"
#Add unique resource name to avoid conflicts
resource_name      = "fmoralk3s"









#| node_os | aws_ami | aws_user | os version |
#| ubuntu | ami-097a2df4ac947655f | ubuntu | Ubuntu 22.04 LTS |
#| ubuntu | ami-0d5bf08bc8017c83b | ubuntu | Ubuntu 20.04 LTS |
#| sles15 | ami-046cd3113e0c1b581 | ec2-user | SLES 15 SP4 |
#| sles15 | ami-021b1f638e534e276 | suse | SLE Micro 5.4 |
#| centos | ami-0f18ced0fd6aae5c2 | centos | Centos 7.9 |
#| centos8 | ami-0382eea882ee397b8 | rocky | Rocky Linux 8.4 |
#| centos8 | ami-05ab2eb74c93eb441 | rocky | Rocky Linux 8.7 |
#| rhel8   | ami-05ab2eb74c93eb441 | rocky | Rocky Linux 8.7 |
#| centos8 | ami-0b2784ccfbe9dc5b5 | core | Fedora CoreOS 38 |
#| oracle8 | ami-054a49e0c0c7fce5c | cloud-user | Oracle Linux 8.7 + SeLinux |
#| rhel | ami-044c6eadf4a0bf8cf | ec2-user | RHEL 7 |
#| rhel8 | ami-0bb95ea8da9bc48e0 | ec2-user | RHEL 8.6 + selinux + FIPS-enabled |
#| rhel8 | ami-0defbb5087b2b63c1 | ec2-user | RHEL 8.7 + selinux + FIPS-enabled |
#| rhel8 | ami-082bf7cc12db545b9 | ec2-user | RHEL 9.2 + selinux + FIPS-enabled |


#### NOT WORKING ####
#node_os            = "rhel8"
#aws_ami            = "ami-082bf7cc12db545b9"
#aws_user           = "ec2-user"

#node_os            = "rhel"
#aws_ami            = "ami-044c6eadf4a0bf8cf"
#aws_user           = "ec2-user"
