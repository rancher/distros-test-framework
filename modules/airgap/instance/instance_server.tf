data "template_file" "is_airgap" {
    template = (var.enable_public_ip == false && var.enable_ipv6 == false) ? true : false
}

data "template_file" "is_ipv6only" {
    template = (var.enable_public_ip == false && var.enable_ipv6 == true) ? true : false
}

resource "aws_instance" "master" {

  depends_on = [ null_resource.prepare_bastion ]

  ami                         = var.aws_ami
  instance_type               = var.ec2_instance_class  
  associate_public_ip_address = false
  ipv6_address_count          = var.enable_ipv6 == true ? 1 : 0
  count                       = var.no_of_server_nodes
  
  root_block_device {
    volume_size          = var.volume_size
    volume_type          = "standard"
  }
  subnet_id              = var.subnets
  availability_zone      = var.availability_zone
  vpc_security_group_ids = [var.sg_id]
  key_name               = var.key_name
  tags = {
    Name                 = "${var.resource_name}-server${count.index + 1}"
  }
  user_data              = <<-EOF
                             #!/bin/bash
                             sudo mkdir -p /etc/rancher/${var.product}
                           EOF  

}

resource "aws_instance" "worker" {

  depends_on = [ null_resource.prepare_bastion ]

  ami                         = var.aws_ami
  instance_type               = var.ec2_instance_class  
  associate_public_ip_address = false
  ipv6_address_count          = var.enable_ipv6 == true ? 1 : 0
  count                       = var.no_of_worker_nodes
  
  root_block_device {
    volume_size          = var.volume_size
    volume_type          = "standard"
  }
  subnet_id              = var.subnets
  availability_zone      = var.availability_zone
  vpc_security_group_ids = [var.sg_id]
  key_name               = var.key_name
  tags = {
    Name                 = "${var.resource_name}-worker${count.index + 1}"
  }
  user_data              = <<-EOF
                             #!/bin/bash
                             sudo mkdir -p /etc/rancher/${var.product}
                           EOF
}

resource "aws_instance" "bastion" {
  ami                         = var.aws_ami
  instance_type               = var.ec2_instance_class  
  associate_public_ip_address = true
  ipv6_address_count          = var.enable_ipv6 == true ? 1 : 0
  count                       = var.no_of_bastion_nodes
  
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
    Name                 = "${var.resource_name}-bastion"
  }

  provisioner "file" {
    source = "../../config/.ssh/aws_key.pem"
    destination = "/tmp/${var.key_name}.pem"
  }

  provisioner "file" {
    source = "setup/get_artifacts.sh"
    destination = "/tmp/get_artifacts.sh"
  }

  provisioner "file" {
    source = "setup/install_product.sh"
    destination = "/tmp/install_product.sh"
  }

  provisioner "file" {
    source = "setup/bastion_prepare.sh"
    destination = "/tmp/bastion_prepare.sh"
  }

  provisioner "file" {
    source = "setup/docker_ops.sh"
    destination = "/tmp/docker_ops.sh"
  }

  provisioner "file" {
    source = "setup/private_registry.sh"
    destination = "/tmp/private_registry.sh"
  }

  provisioner "file" {
    source = "setup/basic-registry"
    destination = "/tmp"
  }
}
resource "null_resource" "prepare_bastion" {

  depends_on = [ aws_instance.bastion[0] ]
  connection {
    type          = "ssh"
    user          = var.aws_user
    host          = aws_instance.bastion[0].public_ip
    private_key   = file(var.access_key)
  }

  provisioner "remote-exec" {
    inline = [<<-EOT
      echo ${aws_instance.bastion[0].public_ip} > /tmp/${var.resource_name}_bastion_ip
      sudo cp /tmp/${var.key_name}.pem /tmp/bastion_prepare.sh /tmp/docker_ops.sh /tmp/get_artifacts.sh /tmp/install_product.sh /tmp/private_registry.sh ~/
      sudo cp -r /tmp/basic-registry ~/
      sudo chmod +x bastion_prepare.sh
      sudo ./bastion_prepare.sh
    EOT
    ]
  }
}
