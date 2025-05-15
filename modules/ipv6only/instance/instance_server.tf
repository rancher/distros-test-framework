resource "aws_instance" "master" {
  depends_on = [ aws_instance.bastion ]

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

resource "aws_instance" "bastion" {
  ami                         = var.aws_ami
  instance_type               = var.ec2_instance_class  
  associate_public_ip_address = true
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
  user_data              = file("scripts/prepare.sh")

  tags = {
    Name                 = "${var.resource_name}-${local.resource_tag}-bastion"
    Team                 = local.resource_tag
  }

  provisioner "local-exec" { 
    command = "aws ec2 wait instance-status-ok --region ${var.region} --instance-ids ${aws_instance.bastion[count.index].id}" 
  }

  provisioner "file" {
    source = "../../config/.ssh/aws_key.pem"
    destination = "/tmp/${var.key_name}.pem"
  }

  provisioner "file" {
    source = "scripts/configure.sh"
    destination = "/tmp/configure.sh"
  }

  provisioner "file" {
    source = "../install/${var.product}_master.sh"
    destination = "/tmp/${var.product}_master.sh"
  }

  provisioner "file" {
    source = "../install/join_${var.product}_master.sh"
    destination = "/tmp/join_${var.product}_master.sh"
  }

  provisioner "file" {
    source = "../install/join_${var.product}_agent.sh"
    destination = "/tmp/join_${var.product}_agent.sh"
  }

  provisioner "remote-exec" {
    inline = [<<-EOT
      sudo cp /tmp/${var.key_name}.pem /tmp/*.sh ~/
    EOT
    ]
  }
}

locals {
  resource_tag =  "distros-qa"
}
