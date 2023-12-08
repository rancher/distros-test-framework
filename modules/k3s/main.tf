module "master" {
   source = "./master"

   # Basic Variables
   rhel_username      = var.rhel_username
   rhel_password      = var.rhel_password

   # AWS Variables
   aws_ami            = var.aws_ami
   aws_user           = var.aws_user
   ec2_instance_class = var.ec2_instance_class
   volume_size        = var.volume_size
   access_key         = var.access_key
   key_name           = var.key_name
   region             = var.region
   vpc_id             = var.vpc_id
   subnets            = var.subnets
   qa_space           = var.qa_space
   availability_zone  = var.availability_zone
   sg_id              = var.sg_id
   enable_public_ip   = var.enable_public_ip
   enable_ipv6        = var.enable_ipv6

   # External Datastore Variables
   db_username         = var.db_username
   db_password         = var.db_password
   db_group_name       = var.db_group_name
   external_db         = var.external_db
   db_instance_class   = var.db_instance_class
   external_db_version = var.external_db_version
   engine_mode         = var.engine_mode
   db_environment      = var.db_environment

   # K3S Variables
   resource_name       = var.resource_name
   node_os             = var.node_os
   no_of_server_nodes  = var.no_of_server_nodes
   k3s_version         = var.k3s_version
   install_mode        = var.install_mode
   create_lb           = var.create_lb
   k3s_channel         = var.k3s_channel
   datastore_type      = var.datastore_type
   server_flags        = var.server_flags
}
module "worker" {
   source     = "./worker"
   dependency = module.master

   # Basic Variables
   rhel_username   = var.rhel_username
   rhel_password   = var.rhel_password

   # AWS Variables
   aws_ami            = var.aws_ami
   aws_user           = var.aws_user
   ec2_instance_class = var.ec2_instance_class
   volume_size        = var.volume_size
   region             = var.region
   vpc_id             = var.vpc_id
   subnets            = var.subnets
   availability_zone  = var.availability_zone
   sg_id              = var.sg_id
   enable_public_ip   = var.enable_public_ip
   enable_ipv6        = var.enable_ipv6
   key_name           = var.key_name
   access_key         = var.access_key
   
   # K3S Variables
   resource_name      = var.resource_name
   node_os            = var.node_os
   no_of_worker_nodes = var.no_of_worker_nodes
   worker_flags       = var.worker_flags
   k3s_version        = var.k3s_version
   install_mode       = var.install_mode
   k3s_channel        = var.k3s_channel
}

module "bastion" {
   source     = "../bastion"
   //dependency = module.master

   # AWS Variables
   aws_ami            = var.aws_ami
   aws_user           = var.aws_user
   ec2_instance_class = var.ec2_instance_class
   region             = var.region
   vpc_id             = var.vpc_id
   bastion_subnets    = var.bastion_subnets
   availability_zone  = var.availability_zone
   sg_id              = var.sg_id
   enable_public_ip   = var.enable_public_ip
   enable_ipv6        = var.enable_ipv6
   key_name           = var.key_name
   access_key         = var.access_key

   resource_name       = var.resource_name
   no_of_bastion_nodes = var.no_of_bastion_nodes
}