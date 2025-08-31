resource "aws_db_instance" "db" {
  count                  = (var.datastore_type == "etcd" || var.external_db == "NULL" ? 0 : (var.external_db != "" && var.external_db != "aurora-mysql" ? 1 : 0))
  identifier             = "${var.resource_name}-${local.resource_tag}-db"
  storage_type           = "gp2"
  allocated_storage      = 20
  engine                 = var.external_db
  engine_version         = var.external_db_version
  instance_class         = var.instance_class
  db_name                = "mydb"
  parameter_group_name   = var.db_group_name
  username               = var.db_username
  password               = var.db_password
  availability_zone      = var.availability_zone
  tags = {
    Environment          = var.environment
    Team                 = local.resource_tag
  }
  skip_final_snapshot    = true
}

resource "aws_rds_cluster" "db" {
  count                  = (var.external_db == "aurora-mysql" && var.datastore_type == "external" ? 1 : 0)
  cluster_identifier     = "${var.resource_name}-${local.resource_tag}-db"
  engine                 = var.external_db
  engine_version         = var.external_db_version
  availability_zones     = [var.availability_zone]
  database_name          = "mydb"
  master_username        = var.db_username
  master_password        = var.db_password
  engine_mode            = var.engine_mode
  tags = {
    Environment          = var.environment
    Team                 = local.resource_tag
  }
  skip_final_snapshot    = true
}

resource "aws_rds_cluster_instance" "db" {
  count                   = (var.external_db == "aurora-mysql" && var.datastore_type == "external" ? 1 : 0)
  cluster_identifier      = aws_rds_cluster.db[0].id
  identifier              = "${var.resource_name}-${local.resource_tag}-instance1"
  instance_class          = var.instance_class
  engine                  = aws_rds_cluster.db[0].engine
  engine_version          = aws_rds_cluster.db[0].engine_version
}

resource "aws_eip" "master_with_eip" {
  count                   = var.create_eip ? 1 : 0
  domain                  = "vpc"
  tags = {
    Name                  = "${var.resource_name}-${local.resource_tag}-server1"
    Team                  = local.resource_tag
  }
}

resource "aws_eip_association" "master_eip_association" {
  count                   = var.create_eip ? 1 : 0
  instance_id             = aws_instance.master.id
  allocation_id           = aws_eip.master_with_eip[count.index].id
  depends_on              = [aws_eip.master_with_eip]
}

resource "aws_instance" "master" {
  ami                         = var.aws_ami
  instance_type               = var.ec2_instance_class
  associate_public_ip_address = var.enable_public_ip
  ipv6_address_count          = var.enable_ipv6 ? 1 : 0

  connection {
    type                 = "ssh"
    user                 = var.aws_user
    host                 = self.public_ip
    private_key          = file(var.access_key)
  }
  root_block_device {
    volume_size          = var.volume_size
    volume_type          = "standard"
  }
  subnet_id              = var.subnets
  availability_zone      = var.availability_zone
  vpc_security_group_ids = [var.sg_id]
  key_name               = var.key_name
  tags = {
    Name                 = "${var.resource_name}-${local.resource_tag}-server1"
    Team                 = local.resource_tag
  }

  provisioner "local-exec" {
    command = "aws ec2 wait instance-status-ok --region ${var.region} --instance-ids ${self.id}"
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
    source      = "../install/node_role.sh"
    destination = "/tmp/node_role.sh"
  }
  provisioner "remote-exec" {
    inline = [
      "chmod +x /tmp/node_role.sh",
      "sudo /tmp/node_role.sh -1 \"${var.role_order}\" ${var.all_role_nodes} ${var.etcd_only_nodes} ${var.etcd_cp_nodes} ${var.etcd_worker_nodes} ${var.cp_only_nodes} ${var.cp_worker_nodes} ${var.product}",
    ]
  }
  provisioner "file" {
    source = "../install/k3s_master.sh"
    destination = "/var/tmp/k3s_master.sh"
  }
  provisioner "file" {
    source = "${path.module}/cis_master_config.yaml"
    destination = "/tmp/cis_master_config.yaml"
  }
  provisioner "file" {
    source = "${path.module}/policy.yaml"
    destination = "/tmp/policy.yaml"
  }
  provisioner "file" {
    source = "${path.module}/audit.yaml"
    destination = "/tmp/audit.yaml"
  }
  provisioner "file" {
    source = "${path.module}/cluster-level-pss.yaml"
    destination = "/tmp/cluster-level-pss.yaml"
  }
  provisioner "file" {
    source = "${path.module}/ingresspolicy.yaml"
    destination = "/tmp/ingresspolicy.yaml"
  }
  provisioner "remote-exec" {
    inline = [
      "chmod +x /var/tmp/k3s_master.sh",
      "sudo /var/tmp/k3s_master.sh ${var.node_os} ${local.fqdn} ${self.public_ip} ${self.private_ip} \"${var.enable_ipv6 ? self.ipv6_addresses[0] : ""}\" ${var.install_mode} ${var.k3s_version} \"${var.k3s_channel}\" ${var.etcd_only_nodes} ${var.datastore_type} \"${data.template_file.test.rendered}\" \"${var.server_flags}\" ${var.username} ${var.password} \"${local.install_or_both}\""
    ]
  }
  provisioner "local-exec" {
    command = "echo \"${var.node_os}\" | grep -q \"slemicro\" && aws ec2 reboot-instances --instance-ids \"${self.id}\" --region \"${var.region}\" && sleep 90 || exit 0"
  }
  provisioner "remote-exec" {
    inline = [
      "sudo /var/tmp/k3s_master.sh ${var.node_os} ${local.fqdn} ${self.public_ip} ${self.private_ip} \"${var.enable_ipv6 ? self.ipv6_addresses[0] : ""}\" ${var.install_mode} ${var.k3s_version} \"${var.k3s_channel}\" ${var.etcd_only_nodes} ${var.datastore_type} \"${data.template_file.test.rendered}\" \"${var.server_flags}\" ${var.username} ${var.password} \"${local.enable_service}\"",
    ]
  }
  //update master_ip file with either eip or public ip
  provisioner "local-exec" {
    command = "echo ${var.create_eip ? aws_eip.master_with_eip[0].public_ip : aws_instance.master.public_ip} >/tmp/${var.resource_name}_master_ip"
  }
  provisioner "local-exec" {
    command = "ssh-keyscan ${aws_instance.master.public_ip} > /tmp/known_hosts || true"
  }
  provisioner "local-exec" {
    command = "scp -i ${var.access_key} ${var.aws_user}@${aws_instance.master.public_ip}:/tmp/nodetoken /tmp/${var.resource_name}_nodetoken"
  }
  provisioner "local-exec" {
    command = "scp -i ${var.access_key} ${var.aws_user}@${aws_instance.master.public_ip}:/tmp/config /tmp/${var.resource_name}_config"
  }
  provisioner "local-exec" {
    command = "scp -i ${var.access_key} ${var.aws_user}@${aws_instance.master.public_ip}:/tmp/joinflags /tmp/${var.resource_name}_joinflags"
  }
  provisioner "local-exec" {
    command = "sed s/127.0.0.1/${var.create_lb ? aws_route53_record.aws_route53[0].fqdn : aws_instance.master.public_ip}/g /tmp/${var.resource_name}_config >/tmp/${var.resource_name}_kubeconfig"
  }
}

resource "null_resource" "master_eip" {
  count = var.create_eip ? 1 : 0
  connection {
    type        = "ssh"
    user        = var.aws_user
    host        = aws_eip.master_with_eip[count.index].public_ip
    private_key = file(var.access_key)
    timeout     = "10m"
  }
  provisioner "remote-exec" {
    inline = [
      "sudo sed -i s/${aws_instance.master.public_ip}/${aws_eip.master_with_eip[count.index].public_ip}/g /etc/rancher/k3s/config.yaml",
      "sudo systemctl restart --no-block k3s"
    ]
  }
  provisioner "local-exec" {
    command = "sed s/127.0.0.1/${aws_eip.master_with_eip[0].public_ip}/g /tmp/${var.resource_name}_config >/tmp/${var.resource_name}_kubeconfig"
  }
   provisioner "local-exec" {
    command = "echo ${aws_eip.master_with_eip[0].public_ip} > /tmp/${var.resource_name}_master_ip"
  }
  provisioner "remote-exec" {
    inline = [
    "echo 'Waiting for eip update to complete'",
    "cloud-init status --wait > /dev/null"
    ]
  }
   depends_on = [aws_instance.master,
                 aws_eip_association.master_eip_association]
}

data "template_file" "test" {
  template   = (var.datastore_type == "etcd" ? "NULL": (var.external_db == "postgres" ? "postgres://${aws_db_instance.db[0].username}:${aws_db_instance.db[0].password}@${aws_db_instance.db[0].endpoint}/${aws_db_instance.db[0].db_name}" : (var.external_db == "aurora-mysql" ? "mysql://${aws_rds_cluster.db[0].master_username}:${aws_rds_cluster.db[0].master_password}@tcp(${aws_rds_cluster.db[0].endpoint})/${aws_rds_cluster.db[0].database_name}" : "mysql://${aws_db_instance.db[0].username}:${aws_db_instance.db[0].password}@tcp(${aws_db_instance.db[0].endpoint})/${aws_db_instance.db[0].db_name}")))
  depends_on = [data.template_file.test_status]
}

data "template_file" "test_status" {
  template = (var.datastore_type == "etcd" ? "NULL": ((var.external_db == "postgres" ? aws_db_instance.db[0].endpoint : (var.external_db == "aurora-mysql" ? aws_rds_cluster_instance.db[0].endpoint : aws_db_instance.db[0].endpoint))))
}

data "local_file" "token" {
  filename   = "/tmp/${var.resource_name}_nodetoken"
  depends_on = [aws_instance.master]
}

resource "aws_eip" "master2_with_eip" {
  count         = var.create_eip ? local.secondary_masters : 0
  domain        = "vpc"
  tags = {
    Name        = "${var.resource_name}-${local.resource_tag}-server${count.index + 2}"
    Team        = local.resource_tag
  }
  depends_on = [aws_eip.master_with_eip ]
}

resource "aws_eip_association" "master2_eip_association" {
  count         = var.create_eip ? local.secondary_masters : 0
  instance_id   = aws_instance.master2-ha[count.index].id
  allocation_id = aws_eip.master2_with_eip[count.index].id
  depends_on    = [aws_eip.master2_with_eip]
}

resource "aws_instance" "master2-ha" {
  ami                         = var.aws_ami
  instance_type               = var.ec2_instance_class
  associate_public_ip_address = var.enable_public_ip
  ipv6_address_count          = var.enable_ipv6 ? 1 : 0
  count = local.secondary_masters
  connection {
    type                 = "ssh"
    user                 = var.aws_user
    host                 = self.public_ip
    private_key          = file(var.access_key)
    timeout="5m"
  }
  root_block_device {
    volume_size          = var.volume_size
    volume_type          = "standard"
  }
  subnet_id              = var.subnets
  availability_zone      = var.availability_zone
  vpc_security_group_ids = [var.sg_id]
  key_name               = var.key_name
  depends_on             = [aws_instance.master]
  tags = {
    Name                 = "${var.resource_name}-${local.resource_tag}-server${count.index + 2}"
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
    source      = "../install/node_role.sh"
    destination = "/tmp/node_role.sh"
  }
  provisioner "remote-exec" {
    inline = [
      "chmod +x /tmp/node_role.sh",
      "sudo /tmp/node_role.sh ${count.index} \"${var.role_order}\" ${var.all_role_nodes} ${var.etcd_only_nodes} ${var.etcd_cp_nodes} ${var.etcd_worker_nodes} ${var.cp_only_nodes} ${var.cp_worker_nodes} ${var.product}",
    ]
  }
  provisioner "file" {
    source = "../install/join_k3s_master.sh"
    destination = "/var/tmp/join_k3s_master.sh"
  }
  provisioner "file" {
    source = "${path.module}/cis_master_config.yaml"
    destination = "/tmp/cis_master_config.yaml"
  }
  provisioner "file" {
    source = "${path.module}/policy.yaml"
    destination = "/tmp/policy.yaml"
  }
  provisioner "file" {
    source = "${path.module}/audit.yaml"
    destination = "/tmp/audit.yaml"
  }
  provisioner "file" {
    source = "${path.module}/cluster-level-pss.yaml"
    destination = "/tmp/cluster-level-pss.yaml"
  }
  provisioner "file" {
    source = "${path.module}/ingresspolicy.yaml"
    destination = "/tmp/ingresspolicy.yaml"
  }
  provisioner "remote-exec" {
    inline = [ <<-EOT
      chmod +x /var/tmp/join_k3s_master.sh
      sudo /var/tmp/join_k3s_master.sh ${var.node_os} ${local.fqdn} ${local.master_node_ip} ${local.node_token} ${self.public_ip} ${self.private_ip} "${var.enable_ipv6 ? self.ipv6_addresses[0] : ""}" ${var.install_mode} ${var.k3s_version} "${var.k3s_channel}" ${var.datastore_type} "${data.template_file.test.rendered}" "${var.server_flags}" ${var.username} ${var.password} "${local.install_or_both}" > /tmp/join_k3s_master.log 2>&1
    EOT
    ]
  }
  provisioner "local-exec" {
    command = "echo \"${var.node_os}\" | grep -q \"slemicro\" && aws ec2 reboot-instances --instance-ids \"${self.id}\" --region \"${var.region}\" && sleep 90 || exit 0"
  }
  provisioner "remote-exec" {
    inline = [ <<-EOT
      sudo /var/tmp/join_k3s_master.sh ${var.node_os} ${local.fqdn} ${local.master_node_ip} ${local.node_token} ${self.public_ip} ${self.private_ip} "${var.enable_ipv6 ? self.ipv6_addresses[0] : ""}" ${var.install_mode} ${var.k3s_version} "${var.k3s_channel}" ${var.datastore_type} "${data.template_file.test.rendered}" "${var.server_flags}" ${var.username} ${var.password} "${local.enable_service}" > /tmp/join_k3s_master.log 2>&1
    EOT
    ]
  }
}

resource "null_resource" "master2_eip" {
  count =   var.create_eip ? local.secondary_masters : 0
  connection {
    type        = "ssh"
    user        = var.aws_user
    host        = aws_eip.master2_with_eip[count.index].public_ip
    private_key = file(var.access_key)
    timeout     = "10m"
  }
  // Replace nodes public ip with elastic ip in the config
  provisioner "remote-exec" {
    inline = [
      "sudo sed -i s/${aws_instance.master2-ha[count.index].public_ip}/${aws_eip.master2_with_eip[count.index].public_ip}/g /etc/rancher/k3s/config.yaml",
      "sudo systemctl restart --no-block k3s"
    ]
  }
  provisioner "local-exec" {
    command = "sed s/127.0.0.1/${aws_eip.master_with_eip[0].public_ip}/g /tmp/${var.resource_name}_config >/tmp/${var.resource_name}_kubeconfig"
  }
  // Update tmp master ip file with eip
  provisioner "local-exec" {
    command = "echo ${aws_eip.master_with_eip[0].public_ip} > /tmp/${var.resource_name}_master_ip"
  }
  provisioner "remote-exec" {
    inline = [
    "echo 'Waiting for eip update to complete'",
    "cloud-init status --wait > /dev/null"
    ]
  }
   depends_on = [aws_eip.master_with_eip,
                 aws_eip_association.master_eip_association]
}

resource "aws_lb_target_group" "aws_tg_80" {
  count              = var.create_lb ? 1 : 0
  port               = 80
  protocol           = "TCP"
  vpc_id             = var.vpc_id
  name               = "${var.resource_name}-${local.resource_tag}-tg-80"
  health_check {
        protocol            = "HTTP"
        port                = "traffic-port"
        path                = "/ping"
        interval            = 10
        timeout             = 6
        healthy_threshold   = 3
        unhealthy_threshold = 3
        matcher             = "200-399"
  }
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_80" {
  count              = var.create_lb ? 1 : 0
  depends_on         = [aws_instance.master]
  target_group_arn   = aws_lb_target_group.aws_tg_80[0].arn
  target_id          = aws_instance.master.id
  port               = 80
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_80_2" {
  count              = var.create_lb ? length(aws_instance.master2-ha) : 0
  depends_on         = [aws_instance.master]
  target_id          = aws_instance.master2-ha[count.index].id
  target_group_arn   = aws_lb_target_group.aws_tg_80[0].arn
  port               = 80

}

resource "aws_lb_target_group" "aws_tg_443" {
  count              = var.create_lb ? 1 : 0
  port               = 443
  protocol           = "TCP"
  vpc_id             = var.vpc_id
  name               = "${var.resource_name}-${local.resource_tag}-tg-443"
  health_check {
        protocol            = "HTTP"
        port                = 80
        path                = "/ping"
        interval            = 10
        timeout             = 6
        healthy_threshold   = 3
        unhealthy_threshold = 3
        matcher             = "200-399"
  }
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_443" {
  count              = var.create_lb ? 1 : 0
  depends_on         = [aws_instance.master]
  target_group_arn   = aws_lb_target_group.aws_tg_443[0].arn
  target_id          = aws_instance.master.id
  port               = 443
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_443_2" {
  count              = var.create_lb ? length(aws_instance.master2-ha) : 0
  depends_on         = [aws_instance.master]
  target_group_arn   = aws_lb_target_group.aws_tg_443[0].arn
  target_id          = aws_instance.master2-ha[count.index].id
  port               = 443
}

resource "aws_lb_target_group" "aws_tg_6443" {
  count              = var.create_lb ? 1 : 0
  port               = 6443
  protocol           = "TCP"
  vpc_id             = var.vpc_id
  name               = "${var.resource_name}-${local.resource_tag}-tg-6443"
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_6443" {
  count              = var.create_lb ? 1 : 0
  depends_on         = [aws_instance.master]
  target_group_arn   = aws_lb_target_group.aws_tg_6443[0].arn
  target_id          = aws_instance.master.id
  port               = 6443
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_6443_2" {
  count              = var.create_lb ? length(aws_instance.master2-ha) : 0
  depends_on         = [aws_instance.master]
  target_group_arn   = aws_lb_target_group.aws_tg_6443[0].arn
  target_id          = aws_instance.master2-ha[count.index].id
  port               = 6443
}

resource "aws_lb" "aws_nlb" {
  count              = var.create_lb ? 1 : 0
  internal           = false
  load_balancer_type = "network"
  subnets            = [var.subnets]
  name               = "${var.resource_name}-${local.resource_tag}-nlb"
}

resource "aws_lb_listener" "aws_nlb_listener_80" {
  count              = var.create_lb ? 1 : 0
  load_balancer_arn  = aws_lb.aws_nlb[0].arn
  port               = "80"
  protocol           = "TCP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.aws_tg_80[0].arn
  }
}

resource "aws_lb_listener" "aws_nlb_listener_443" {
  count              = var.create_lb ? 1 : 0
  load_balancer_arn  = aws_lb.aws_nlb[0].arn
  port               = "443"
  protocol           = "TCP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.aws_tg_443[0].arn
  }
}

resource "aws_lb_listener" "aws_nlb_listener_6443" {
  count              = var.create_lb ? 1 : 0
  load_balancer_arn  = aws_lb.aws_nlb[0].arn
  port               = "6443"
  protocol           = "TCP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.aws_tg_6443[0].arn
  }
}

resource "aws_route53_record" "aws_route53" {
  count              = var.create_lb ? 1 : 0
  depends_on         = [aws_lb_listener.aws_nlb_listener_6443]
  zone_id            = data.aws_route53_zone.selected.zone_id
  name               = "${var.resource_name}-${local.resource_tag}-r53"
  type               = "CNAME"
  ttl                = "300"
  records            = [aws_lb.aws_nlb[0].dns_name]
}

data "aws_route53_zone" "selected" {
  name               = var.qa_space
  private_zone       = false
}

resource "null_resource" "update_kubeconfig" {
count =  var.create_eip ? 0: local.total_server_count
depends_on = [aws_instance.master, aws_instance.master2-ha]

  provisioner "local-exec" {
    command = "ssh-keyscan ${count.index == 0 ? aws_instance.master.public_ip : aws_instance.master2-ha[count.index - 1].public_ip} >> /tmp/known_hosts || true"
  }
  provisioner "local-exec" {
    command    = "scp -i ${var.access_key} ${var.aws_user}@${count.index == 0 ? aws_instance.master.public_ip : aws_instance.master2-ha[count.index - 1].public_ip}:/var/tmp/.control-plane /tmp/${var.resource_name}_control_plane_${count.index}"
    on_failure = continue
  }
  provisioner "local-exec" {
    command    = "test -f /tmp/${var.resource_name}_control_plane_${count.index} && grep '6444' /tmp/${var.resource_name}_config && sed s/127.0.0.1:6444/\"${count.index == 0 ? local.serverIP : aws_instance.master2-ha[count.index - 1].public_ip}:6443\"/g /tmp/${var.resource_name}_config >/tmp/${var.resource_name}_kubeconfig"
    on_failure = continue
  }

  provisioner "local-exec" {
    command    = "test -f /tmp/${var.resource_name}_control_plane_${count.index} && grep '6443' /tmp/${var.resource_name}_config && sed s/127.0.0.1:6443/\"${count.index == 0 ? local.serverIP : aws_instance.master2-ha[count.index - 1].public_ip}:6443\"/g /tmp/${var.resource_name}_config >/tmp/${var.resource_name}_kubeconfig"
    on_failure = continue
  }
}

locals {
  serverIP                = var.create_lb ? aws_route53_record.aws_route53[0].fqdn : aws_instance.master.public_ip
  total_server_count      = var.no_of_server_nodes + var.etcd_only_nodes + var.etcd_cp_nodes + var.etcd_worker_nodes + var.cp_only_nodes + var.cp_worker_nodes
  master_node_ip          = var.create_eip ? aws_eip.master_with_eip[0].public_ip : aws_instance.master.public_ip
  secondary_masters       = var.no_of_server_nodes + var.etcd_only_nodes + var.etcd_cp_nodes + var.etcd_worker_nodes + var.cp_only_nodes + var.cp_worker_nodes - 1

  fqdn                    = var.create_lb ? aws_route53_record.aws_route53[0].fqdn : var.create_eip ? aws_eip.master_with_eip[0].public_ip : "fake.fqdn.value"
  node_token              = trimspace(data.local_file.token.content)
  resource_tag            =  "distros-qa"
 }
