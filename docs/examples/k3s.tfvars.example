## K3S variables -- Fill these in with your desired values.

k3s_version    = "v1.27.4+k3s1"
k3s_channel    = "latest"
server_flags   = "protect-kernel-defaults: true\n"
worker_flags   = "protect-kernel-defaults: true\n"
no_of_server_nodes = 3
no_of_worker_nodes = 1
resource_name  = "<prefix_name_for_your_resources>"
key_name       = "jenkins-rke-validation"
access_key     = "/go/src/github.com/rancher/distros-test-framework/config/.ssh/aws_key.pem"

install_mode   = "INSTALL_K3S_VERSION"
# INSTALL_K3S_VERSION or INSTALL_K3S_COMMIT

create_lb      = false
arch           = "amd64"

## Custom Vars
node_os            = "sles15"
aws_ami            = "<ami-id>"
aws_user           = "ec2-user"
# This is also known as an "all-roles" node
#no_of_server_nodes = 3
#no_of_worker_nodes = 1
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
qa_space           = "qa.rancher.space"
ec2_instance_class = "t3a.medium"
vpc_id             = "<vpc_id>"
subnets            = "<subnet_id>"
availability_zone  = "us-east-2a"
sg_id              = "<sg_id>"
iam_role           = "<iam_role>"
volume_size        = "20"

datastore_type       = "etcd"

##############  external db variables  #################

# to use external db set datastore_type to "" 

db_username           = "<db_user>"
db_password           = "<db_password>"

# mysql
external_db           = "mysql"
external_db_version   = "8.0.32"
instance_class        = "db.t3.micro"
db_group_name         = "default.mysql8.0"


#external_db          = "postgres"
#external_db_version  = "14.6"
#db_group_name        = "default.postgres14"
#instance_class       = "db.t3.micro"

#aurora-mysql
#external_db          = "aurora-mysql"
#external_db_version  = "5.7.mysql_aurora.2.11.2"
#instance_class       = "db.t3.medium"
#db_group_name        = "default.aurora-mysql5.7"
environment           = "dev"
engine_mode           = "provisioned"

## mariadb
#external_db          = "mariadb"
#external_db_version  = "10.6.11"
#instance_class       = "db.t3.medium"
#db_group_name        = "default.mariadb10.6"
