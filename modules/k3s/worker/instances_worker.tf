resource "aws_instance" "worker" {
  depends_on = [
    var.dependency
  ]
  ami                         = var.aws_ami
  instance_type               = var.ec2_instance_class
  associate_public_ip_address = var.enable_public_ip
  ipv6_address_count          = var.enable_ipv6 ? 1 : 0
  count                       = var.no_of_worker_nodes
  connection {
    type                 = "ssh"
    user                 = var.aws_user
    host                 = self.public_ip
    private_key          = file(var.access_key)
  }
  root_block_device {
    volume_size = var.volume_size
    volume_type = "standard"
  }
  subnet_id              = var.subnets
  availability_zone      = var.availability_zone
  vpc_security_group_ids = [var.sg_id]
  key_name               = var.key_name
  tags = {
    Name                 = "${var.resource_name}-${local.resource_tag}-worker${count.index + 1}"
    Team                 = local.resource_tag
  }
  provisioner "remote-exec" {
    inline = [
      "echo \"${var.node_os}\" | grep -q \"slemicro\" && sudo transactional-update setup-selinux || exit 0",
    ]
  }

  provisioner "local-exec" {
    command = "echo \"${var.node_os}\" | grep -q \"slemicro\" && aws ec2 reboot-instances --instance-ids \"${self.id}\" --region \"${var.region}\" && sleep 90 || exit 0"
  }
  provisioner "file" {
    source               = "../install/join_k3s_agent.sh"
    destination          = "/var/tmp/join_k3s_agent.sh"
  }
  provisioner "file" {
    source               = "${path.module}/cis_worker_config.yaml"
    destination          = "/tmp/cis_worker_config.yaml"
  }
  provisioner "remote-exec" {
    inline = [
      "chmod +x /var/tmp/join_k3s_agent.sh",
      "sudo /var/tmp/join_k3s_agent.sh ${var.node_os} ${local.master_ip} ${local.node_token} ${self.public_ip} ${self.private_ip} \"${var.enable_ipv6 ? self.ipv6_addresses[0] : ""}\" ${var.install_mode} ${var.k3s_version} \"${var.k3s_channel}\" \"${var.worker_flags}\" ${var.username} ${var.password} \"${local.install_or_both}\"",
    ]
  }

  provisioner "local-exec" {
    command = "echo \"${var.node_os}\" | grep -q \"slemicro\" && aws ec2 reboot-instances --instance-ids \"${self.id}\" --region \"${var.region}\" && sleep 90 || exit 0"
  }
  provisioner "remote-exec" {
    inline = [
      "sudo /var/tmp/join_k3s_agent.sh ${var.node_os} ${local.master_ip} ${local.node_token} ${self.public_ip} ${self.private_ip} \"${var.enable_ipv6 ? self.ipv6_addresses[0] : ""}\" ${var.install_mode} ${var.k3s_version} \"${var.k3s_channel}\" \"${var.worker_flags}\" ${var.username} ${var.password} \"${local.enable_service}\"",
    ]
  }
}

resource "aws_eip_association" "worker_eip_association" {
  count              = var.create_eip ? length(aws_instance.worker) : 0
  instance_id        = aws_instance.worker[count.index].id
  allocation_id      = aws_eip.worker_with_eip[count.index].id
}

resource "aws_eip" "worker_with_eip" {
  count              = var.create_eip ? length(aws_instance.worker) : 0
  domain             = "vpc"
  tags = {
    Name                 = "${var.resource_name}-${local.resource_tag}-worker${count.index + 1}"
    Team                 = local.resource_tag
  }
}

data "local_file" "master_ip" {
  depends_on         = [var.dependency]
  filename           = "/tmp/${var.resource_name}_master_ip"
}

data "local_file" "token" {
  depends_on        = [var.dependency]
  filename          = "/tmp/${var.resource_name}_nodetoken"
}

resource "null_resource" "worker_eip" {
  count         = var.create_eip ? length(aws_instance.worker) : 0
  connection {
    type        = "ssh"
    user        = var.aws_user
    host        = aws_eip.worker_with_eip[count.index].public_ip
    private_key = file(var.access_key)
    timeout     = "25m"
  }
  provisioner "remote-exec" {
    inline = [
      "sudo sed -i s/${aws_instance.worker[count.index].public_ip}/${aws_eip.worker_with_eip[count.index].public_ip}/g /etc/rancher/k3s/config.yaml",
      "sudo systemctl restart --no-block k3s-agent"
    ]
  }
  provisioner "remote-exec" {
    inline = [
    "echo 'Waiting for eip update to complete'",
    "cloud-init status --wait > /dev/null"
    ]
  }
  depends_on = [aws_eip.worker_with_eip,
                 aws_eip_association.worker_eip_association]
}

locals {
  master_ip       = trimspace(data.local_file.master_ip.content)
  node_token      = trimspace(data.local_file.token.content)

  resource_tag    =  "distros-qa"
}
