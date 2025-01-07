# Server Nodes
module "master" {
  source = "./master"

  # Basic variables
  node_os              = var.node_os
  no_of_server_nodes   = var.no_of_server_nodes
  create_lb            = var.create_lb
  username             = var.username
  password             = var.password
  all_role_nodes       = var.no_of_server_nodes
  etcd_only_nodes      = var.etcd_only_nodes
  etcd_cp_nodes        = var.etcd_cp_nodes
  etcd_worker_nodes    = var.etcd_worker_nodes
  cp_only_nodes        = var.cp_only_nodes
  cp_worker_nodes      = var.cp_worker_nodes
  optional_files       = var.optional_files

  # AWS variables
  key_name             = var.key_name
  access_key           = var.access_key
  aws_ami              = var.aws_ami
  availability_zone    = var.availability_zone
  ec2_instance_class   = var.ec2_instance_class
  aws_user             = var.aws_user
  iam_role             = var.iam_role
  volume_size          = var.volume_size
  region               = var.region
  hosted_zone          = var.hosted_zone
  sg_id                = var.sg_id
  resource_name        = var.resource_name
  vpc_id               = var.vpc_id
  subnets              = var.subnets
  datastore_type       = var.datastore_type
  db_username          = var.db_username
  db_password          = var.db_password
  db_group_name        = var.db_group_name
  external_db          = var.external_db
  instance_class       = var.instance_class
  external_db_version  = var.external_db_version
  engine_mode          = var.engine_mode
  environment          = var.environment
  create_eip           = var.create_eip

  # RKE2 variables
  install_version      = var.install_version
  install_mode         = var.install_mode
  install_method       = var.install_method
  install_channel      = var.install_channel
  server_flags         = var.server_flags
  split_roles          = var.split_roles
  role_order           = var.role_order
}

# Agent Nodes
module "worker" {
  source     = "./worker"
  dependency = module.master

  # Basic variables
  node_os            = var.node_os
  no_of_worker_nodes = var.no_of_worker_nodes
  username           = var.username
  password           = var.password

  # AWS variables
  access_key         = var.access_key
  key_name           = var.key_name
  availability_zone  = var.availability_zone
  aws_ami            = var.aws_ami
  aws_user           = var.aws_user
  ec2_instance_class = var.ec2_instance_class
  volume_size        = var.volume_size
  iam_role           = var.iam_role
  region             = var.region
  resource_name      = var.resource_name
  sg_id              = var.sg_id
  subnets            = var.subnets
  vpc_id             = var.vpc_id
  create_eip         = var.create_eip


  # RKE2 variables
  install_version    = var.install_version
  install_mode       = var.install_mode
  install_method     = var.install_method
  install_channel    = var.install_channel
  worker_flags       = var.worker_flags
}

module "windows_worker" {
  source     = "./windows_worker"
  dependency = module.master

  # Basic variables
  no_of_worker_nodes = var.no_of_windows_worker_nodes
  username           = var.username
  password           = var.password

  # AWS variables
  access_key         = var.access_key
  key_name           = var.key_name
  availability_zone  = var.availability_zone
  aws_ami            = var.windows_aws_ami
  aws_user           = "Administrator"
  ec2_instance_class = var.windows_ec2_instance_class
  iam_role           = var.iam_role
  region             = var.region
  resource_name      = var.resource_name
  sg_id              = var.sg_id
  subnets            = var.subnets
  vpc_id             = var.vpc_id

  # RKE2 variables
  install_version    = var.install_version
  install_mode       = var.install_mode
}
