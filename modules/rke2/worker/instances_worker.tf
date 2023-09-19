resource "aws_instance" "worker" {
  depends_on = [
    var.dependency
  ]
  ami = var.aws_ami
  instance_type = var.ec2_instance_class
  count = var.no_of_worker_nodes
  iam_instance_profile = var.iam_role
  connection {
    type = "ssh"
    user = var.aws_user
    host = self.public_ip
    private_key = file(var.access_key)
  }
  root_block_device {
    volume_size = var.volume_size
    volume_type = "standard"
  }
  subnet_id = var.subnets
  availability_zone = var.availability_zone
  vpc_security_group_ids = [
    var.sg_id
  ]
  key_name = var.key_name
  tags = {
    Name = "${var.resource_name}-worker"
    "kubernetes.io/cluster/clusterid" = "owned"
  }
  provisioner "file" {
    source = "../install/join_rke2_agent.sh"
    destination = "/tmp/join_rke2_agent.sh"
  }
  provisioner "remote-exec" {
    inline = [
      "chmod +x /tmp/join_rke2_agent.sh",
      "sudo /tmp/join_rke2_agent.sh ${var.node_os} ${local.master_ip} \"${local.node_token}\" ${var.rke2_version} ${self.public_ip} ${var.rke2_channel} \"${var.worker_flags}\" ${var.install_mode} ${var.username} ${var.password} \"${var.install_method}\"",
    ]
  }
}

locals {
        eip_index = { for i, v in aws_instance.worker : tonumber(i)  => v.id if var.create_eip}      
}

resource "aws_eip" "worker-eip" {
  depends_on = [var.dependency]
  for_each = local.eip_index
  vpc = true
  tags = {
      Name ="${var.resource_name}-worker-${each.key}"
    }
}

resource "aws_eip_association" "worker-association" {
        for_each = local.eip_index 
        instance_id = aws_instance.worker[each.key].id
        allocation_id = aws_eip.worker-eip[each.key].id
        depends_on = [aws_instance.worker]
} 

resource "null_resource" "worker_eip" {

  for_each = local.eip_index
  connection {
    type = "ssh"
    user = var.aws_user
    host = aws_eip.worker-eip[each.key].public_ip
    private_key = "${file(var.access_key)}"
  }

  provisioner "remote-exec" { 
    inline = [
      "sudo sed -i s/${local.master_ip_prev}/${local.master_ip}/g /etc/rancher/rke2/config.yaml",
      "sudo sed -i s/-ip:.*/\"-ip: ${aws_eip.worker-eip[each.key].public_ip}\"/g /etc/rancher/rke2/config.yaml",
      "sudo systemctl restart rke2-agent"
    ]
  }

provisioner "local-exec" {
    command    = "sed s/127.0.0.1/\"${local.master_ip}\"/g /tmp/${var.resource_name}_config >/tmp/${var.resource_name}_kubeconfig"
    on_failure = continue
  }

   depends_on = [aws_instance.worker, 
                 aws_eip_association.worker-association]
}

data "local_file" "master_ip" {
  depends_on = [var.dependency]
  filename = "/tmp/${var.resource_name}_master_ip"
}

locals {
  master_ip = trimspace("${data.local_file.master_ip.content}")
}

data "local_file" "master_ip_prev" {
  depends_on = [var.dependency]
  filename = "/tmp/${var.resource_name}_master_ip_prev"
}

locals {
  master_ip_prev = trimspace("${data.local_file.master_ip_prev.content}")
}

data "local_file" "token" {
  depends_on = [var.dependency]
  filename = "/tmp/${var.resource_name}_nodetoken"
}

locals {
  node_token = trimspace("${data.local_file.token.content}")
}

resource "null_resource" "stop_resource" {
  count = var.create_eip ? 1 : 0
  depends_on = [null_resource.worker_eip]
   provisioner "local-exec" {
   command = "chmod +x ../install/rke2_stop_start_instance.sh"
  }
  provisioner "local-exec" {
    command = "../install/rke2_stop_start_instance.sh stop ${var.resource_name}"
  }
}

resource "time_sleep" "wait_for_stop" {
  count = var.create_eip ? 1 : 0
  create_duration = "400s"
  depends_on = [null_resource.stop_resource]
}

resource "null_resource" "start_server1_server2" {
  count = var.create_eip ? 1 : 0
  depends_on = [time_sleep.wait_for_stop]
  provisioner "local-exec" {
    command = "../install/rke2_stop_start_instance.sh start_s1_s2 ${var.resource_name}"
  }
}

resource "null_resource" "start_master_worker" {
  count = var.create_eip ? 1 : 0
  depends_on = [null_resource.start_server1_server2]
  provisioner "local-exec" {
    command = "../install/rke2_stop_start_instance.sh start_master_worker ${var.resource_name}"
  }
}