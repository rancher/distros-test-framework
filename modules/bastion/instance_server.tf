resource "aws_instance" "bastion" {
  ami                         = var.aws_ami
  instance_type               = var.ec2_instance_class  
  associate_public_ip_address = true
  ipv6_address_count          = var.enable_ipv6 ? 1 : 0
  count                       = var.no_of_bastion_nodes == 0 ? 0 : 1
  
  connection {
    type          = "ssh"
    user          = var.aws_user
    host          = self.public_ip
    private_key   = file(var.access_key)
  }
  root_block_device {
    volume_size          = var.volume_size
    volume_type          = "standard"
  }
  subnet_id              = var.bastion_subnets
  availability_zone      = var.availability_zone
  vpc_security_group_ids = [var.sg_id]
  key_name               = var.key_name
  tags = {
    Name                 = "${var.resource_name}-${local.resource_tag}-bastion-server"
  }

  provisioner "file" {
    source = "../../config/.ssh/aws_key.pem"
    destination = "/tmp/jenkins_rke_validation.pem"
  }

  provisioner "local-exec" {
    command = "echo ${aws_instance.bastion[0].public_ip} > /tmp/${var.resource_name}_bastion_ip"
  }
}

locals {
  resource_tag =  "distros-qa"
}
