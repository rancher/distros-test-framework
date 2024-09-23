module "instance" {
   source     = "./instance"

   # Basic Variables
   username   = var.username
   password   = var.password

   # AWS Variables
   aws_ami             = var.aws_ami
   aws_user            = var.aws_user
   ec2_instance_class  = var.ec2_instance_class
   region              = var.region
   vpc_id              = var.vpc_id
   bastion_subnets     = var.bastion_subnets
   subnets             = var.subnets
   availability_zone   = var.availability_zone
   sg_id               = var.sg_id
   volume_size         = var.volume_size
   enable_public_ip    = var.enable_public_ip
   enable_ipv6         = var.enable_ipv6
   key_name            = var.key_name
   access_key          = var.access_key
   arch                = var.arch

   resource_name       = var.resource_name
   no_of_bastion_nodes = var.no_of_bastion_nodes
   product             = var.product
   install_mode        = var.install_mode
   install_version     = var.install_version
   channel             = var.channel
   install_method      = var.install_method
   no_of_server_nodes  = var.no_of_server_nodes
   no_of_worker_nodes  = var.no_of_worker_nodes
}
