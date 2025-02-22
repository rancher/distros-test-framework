resource "aws_instance" "master" {
  depends_on = [ null_resource.prepare_bastion ]

  ami                         = var.aws_ami
  instance_type               = var.ec2_instance_class  
  associate_public_ip_address = false
  ipv6_address_count          = var.enable_ipv6 ? 1 : 0
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
    Name                 = "${var.resource_name}-${local.resource_tag}-server${count.index + 1}"
    Team                 = local.resource_tag
  } 

  provisioner "local-exec" { 
    command = "aws ec2 wait instance-status-ok --region ${var.region} --instance-ids ${self.id}" 
  }
}

resource "aws_instance" "worker" {
  depends_on = [ aws_instance.master ]

  ami                         = var.aws_ami
  instance_type               = var.ec2_instance_class  
  associate_public_ip_address = false
  ipv6_address_count          = var.enable_ipv6 ? 1 : 0
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
    Name                 = "${var.resource_name}-${local.resource_tag}-worker${count.index + 1}"
    Team                 = local.resource_tag
  }

  provisioner "local-exec" { 
    command = "aws ec2 wait instance-status-ok --region ${var.region} --instance-ids ${self.id}" 
  }
}

resource "aws_instance" "windows_worker" {
  depends_on = [ aws_instance.master ]

  ami                         = var.windows_aws_ami
  instance_type               = var.windows_ec2_instance_class  
  associate_public_ip_address = false
  ipv6_address_count          = var.enable_ipv6 ? 1 : 0
  count                       = var.no_of_windows_worker_nodes
  
  root_block_device {
    volume_size          = 50
    volume_type          = "standard"
  }
  subnet_id              = var.subnets
  availability_zone      = var.availability_zone
  vpc_security_group_ids = [var.sg_id]
  key_name               = var.key_name
  get_password_data      = true
  tags = {
    Name                 = "${var.resource_name}-${local.resource_tag}-windows-worker${count.index + 1}"
    Team                 = local.resource_tag
  }

  provisioner "local-exec" { 
    command = "aws ec2 wait instance-status-ok --region ${var.region} --instance-ids ${self.id}" 
  }
}

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
    Name                 = "${var.resource_name}-${local.resource_tag}-bastion"
    Team                 = local.resource_tag
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
    source = "setup/images_ptpv.sh"
    destination = "/tmp/images_ptpv.sh"
  }
  provisioner "file" {
    source = "setup/private_registry.sh"
    destination = "/tmp/private_registry.sh"
  }

  provisioner "file" {
    source = "setup/system_default_registry.sh"
    destination = "/tmp/system_default_registry.sh"
  }

  provisioner "file" {
    source = "setup/windows_install.ps1"
    destination = "/tmp/windows_install.ps1"
  }
  provisioner "file" {
    source = "setup/basic-registry"
    destination = "/tmp"
  }
}

resource "null_resource" "prepare_bastion" {
  depends_on = [ aws_instance.bastion ]
  connection {
    type          = "ssh"
    user          = var.aws_user
    host          = aws_instance.bastion[0].public_ip
    private_key   = file(var.access_key)
  }

  provisioner "remote-exec" {
    inline = [<<-EOT
      sudo cp /tmp/${var.key_name}.pem /tmp/*.sh /tmp/*.ps1 ~/
      sudo cp -r /tmp/basic-registry ~/
      sudo chmod +x bastion_prepare.sh
      sudo ./bastion_prepare.sh "${var.no_of_windows_worker_nodes}"
    EOT
    ]
  }
}

locals {
  resource_tag =  "distros-qa"
}
