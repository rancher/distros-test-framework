resource "aws_instance" "server" {
  ami                  = var.aws_ami
  instance_type        = var.ec2_instance_class
  iam_instance_profile = var.iam_role
  connection {
    type        = "ssh"
    user        = var.aws_user
    host        = self.public_ip
    private_key = file(var.access_key)
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
    Name                              = "${var.resource_name}-server"
    "kubernetes.io/cluster/clusterid" = "owned"
  }
  provisioner "file" {
    source      = "../install/optional_write_files.sh"
    destination = "/tmp/optional_write_files.sh"
  }
  provisioner "remote-exec" {
    inline = [
      "chmod +x /tmp/optional_write_files.sh",
      "sudo /tmp/optional_write_files.sh \"${var.optional_files}\"",
    ]
  }
  provisioner "file" {
    source      = "../install/rke2_node_role.sh"
    destination = "/tmp/rke2_node_role.sh"
  }
  provisioner "remote-exec" {
    inline = [
      "chmod +x /tmp/rke2_node_role.sh",
      "sudo /tmp/rke2_node_role.sh -1 \"${var.role_order}\" ${var.all_role_nodes} ${var.etcd_only_nodes} ${var.etcd_cp_nodes} ${var.etcd_worker_nodes} ${var.cp_only_nodes} ${var.cp_worker_nodes}",
    ]
  }
  provisioner "file" {
    source      = "../install/rke2_server.sh"
    destination = "/tmp/rke2_server.sh"
  }
  provisioner "remote-exec" {
    inline = [
      "chmod +x /tmp/rke2_server.sh",
      "sudo /tmp/rke2_server.sh ${var.node_os} ${var.create_lb ? aws_route53_record.aws_route53[0].fqdn : "fake.fqdn.value"} ${var.rke2_version} ${self.public_ip} ${var.rke2_channel} \"${var.server_flags}\" ${var.install_mode} ${var.username} ${var.password} \"${var.install_method}\"",
    ]
  }
  provisioner "local-exec" {
    command = "echo ${aws_instance.server.public_ip} >/tmp/${var.resource_name}_server_ip"
  }
  provisioner "local-exec" {
    command = "scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i ${var.access_key} ${var.aws_user}@${aws_instance.server.public_ip}:/tmp/nodetoken /tmp/${var.resource_name}_nodetoken"
  }
  provisioner "local-exec" {
    command = "scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i ${var.access_key} ${var.aws_user}@${aws_instance.server.public_ip}:/tmp/config /tmp/${var.resource_name}_config"
  }
  provisioner "local-exec" {
    command = "scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i ${var.access_key} ${var.aws_user}@${aws_instance.server.public_ip}:/tmp/joinflags /tmp/${var.resource_name}_joinflags"
  }
}

resource "aws_instance" "server2" {
  ami                  = var.aws_ami
  instance_type        = var.ec2_instance_class
  iam_instance_profile = var.iam_role
  count                = var.no_of_server_nodes + var.etcd_only_nodes + var.etcd_cp_nodes + var.etcd_worker_nodes + var.cp_only_nodes + var.cp_worker_nodes - 1
  connection {
    type        = "ssh"
    user        = var.aws_user
    host        = self.public_ip
    private_key = file(var.access_key)
  }
  root_block_device {
    volume_size = var.volume_size
    volume_type = "standard"
  }
  subnet_id              = var.subnets
  availability_zone      = var.availability_zone
  vpc_security_group_ids = [var.sg_id]
  key_name               = var.key_name
  tags  =                {
    Name                 = "${var.resource_name}-server${count.index + 1}"
    "kubernetes.io/cluster/clusterid" = "owned"
  }
  depends_on = [aws_instance.server]
  provisioner "file" {
    source      = "../install/optional_write_files.sh"
    destination = "/tmp/optional_write_files.sh"
  }
  provisioner "remote-exec" {
    inline = [
      "chmod +x /tmp/optional_write_files.sh",
      "sudo /tmp/optional_write_files.sh \"${var.optional_files}\"",
    ]
  }
  provisioner "file" {
    source      = "../install/rke2_node_role.sh"
    destination = "/tmp/rke2_node_role.sh"
  }
  provisioner "remote-exec" {
    inline = [
      "chmod +x /tmp/rke2_node_role.sh",
      "sudo /tmp/rke2_node_role.sh ${count.index} \"${var.role_order}\" ${var.all_role_nodes} ${var.etcd_only_nodes} ${var.etcd_cp_nodes} ${var.etcd_worker_nodes} ${var.cp_only_nodes} ${var.cp_worker_nodes}",
    ]
  }
  provisioner "file" {
    source      = "../install/join_rke2_server.sh"
    destination = "/tmp/join_rke2_server.sh"
  }
  provisioner "remote-exec" {
    inline = [
      "chmod +x /tmp/join_rke2_server.sh",
      "sudo /tmp/join_rke2_server.sh ${var.node_os} ${var.create_lb ? aws_route53_record.aws_route53[0].fqdn : aws_instance.server.public_ip} ${aws_instance.server.public_ip} ${local.node_token} ${var.rke2_version} ${self.public_ip} ${var.rke2_channel} \"${var.server_flags}\" ${var.install_mode} ${var.username} ${var.password} \"${var.install_method}\"",
    ]
  }
}

data "local_file" "token" {
  filename   = "/tmp/${var.resource_name}_nodetoken"
  depends_on = [aws_instance.server]
}

locals {
  node_token = trimspace(data.local_file.token.content)
}

resource "random_string" "suffix" {
  length = 4
  upper = false
  special = false
}

locals {
  random_string =  random_string.suffix.result
}

resource "local_file" "server_ips" {
  content  = join(",", aws_instance.server.*.public_ip, aws_instance.server2.*.public_ip)
  filename = "/tmp/${var.resource_name}_server_ips"
}

resource "aws_lb_target_group" "aws_tg_6443" {
  port     = 6443
  protocol = "TCP"
  vpc_id   = var.vpc_id
  name     = "${var.resource_name}${local.random_string}-tg-6443"
  count    = var.create_lb ? 1 : 0
}

resource "aws_lb_target_group" "aws_tg_9345" {
  port     = 9345
  protocol = "TCP"
  vpc_id   = var.vpc_id
  name     = "${var.resource_name}${local.random_string}-tg-9345"
  count    = var.create_lb ? 1 : 0
}

resource "aws_lb_target_group" "aws_tg_80" {
  port     = 80
  protocol = "TCP"
  vpc_id   = var.vpc_id
  name     = "${var.resource_name}${local.random_string}-tg-80"
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
  count = var.create_lb ? 1 : 0
}

resource "aws_lb_target_group" "aws_tg_443" {
  port     = 443
  protocol = "TCP"
  vpc_id   = var.vpc_id
  name     = "${var.resource_name}${local.random_string}-tg-443"
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
  count = var.create_lb ? 1 : 0
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_6443" {
  target_group_arn = aws_lb_target_group.aws_tg_6443[0].arn
  target_id        = aws_instance.server.id
  port             = 6443
  depends_on       = [aws_instance.server]
  count            = var.create_lb ? 1 : 0
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_6443_2" {
  target_group_arn = aws_lb_target_group.aws_tg_6443[0].arn
  count            = var.create_lb ? length(aws_instance.server2) : 0
  target_id        = aws_instance.server2[count.index].id
  depends_on       = [aws_instance.server2]
  port             = 6443
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_9345" {
  target_group_arn = aws_lb_target_group.aws_tg_9345[0].arn
  target_id        = aws_instance.server.id
  port             = 9345
  depends_on       = [aws_instance.server]
  count            = var.create_lb ? 1 : 0
}
resource "aws_lb_target_group_attachment" "aws_tg_attachment_9345_2" {
  target_group_arn = aws_lb_target_group.aws_tg_9345[0].arn
  count            = var.create_lb ? length(aws_instance.server2) : 0
  target_id        = aws_instance.server2[count.index].id
  depends_on       = [aws_instance.server2]
  port             = 9345
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_80" {
  target_group_arn = aws_lb_target_group.aws_tg_80[0].arn
  target_id        = aws_instance.server.id
  port             = 80
  depends_on       = [aws_instance.server]
  count            = var.create_lb ? 1 : 0
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_80_2" {
  target_group_arn = aws_lb_target_group.aws_tg_80[0].arn
  count            = var.create_lb ? length(aws_instance.server2) : 0
  target_id        = aws_instance.server2[count.index].id
  port             = 80
  depends_on       = [aws_instance.server]
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_443" {
  target_group_arn = aws_lb_target_group.aws_tg_443[0].arn
  target_id        = aws_instance.server.id
  port             = 443
  depends_on       = [aws_instance.server]
  count            = var.create_lb ? 1 : 0
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_443_2" {
  target_group_arn = aws_lb_target_group.aws_tg_443[0].arn
  count            = var.create_lb ? length(aws_instance.server2) : 0
  target_id        = aws_instance.server2[count.index].id
  port             = 443
  depends_on       = [aws_instance.server]
}

resource "aws_lb" "aws_nlb" {
  internal           = false
  load_balancer_type = "network"
  subnets            = [var.subnets]
  name               = "${var.resource_name}${local.random_string}-nlb"
  count              = var.create_lb ? 1 : 0
}

resource "aws_lb_listener" "aws_nlb_listener_6443" {
  load_balancer_arn = aws_lb.aws_nlb[0].arn
  port              = "6443"
  protocol          = "TCP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.aws_tg_6443[0].arn
  }
  count = var.create_lb ? 1 : 0
}

resource "aws_lb_listener" "aws_nlb_listener_9345" {
  load_balancer_arn = aws_lb.aws_nlb[0].arn
  port              = "9345"
  protocol          = "TCP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.aws_tg_9345[0].arn
  }
  count = var.create_lb ? 1 : 0
}

resource "aws_lb_listener" "aws_nlb_listener_80" {
  load_balancer_arn = aws_lb.aws_nlb[0].arn
  port              = "80"
  protocol          = "TCP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.aws_tg_80[0].arn
  }
  count = var.create_lb ? 1 : 0
}

resource "aws_lb_listener" "aws_nlb_listener_443" {
  load_balancer_arn = aws_lb.aws_nlb[0].arn
  port              = "443"
  protocol          = "TCP"
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.aws_tg_443[0].arn
  }
  count = var.create_lb ? 1 : 0
}

resource "aws_route53_record" "aws_route53" {
  zone_id = data.aws_route53_zone.selected.zone_id
  name    = "${var.resource_name}${local.random_string}-r53"
  type    = "CNAME"
  ttl     = "300"
  records = [aws_lb.aws_nlb[0].dns_name]
  count   = var.create_lb ? 1 : 0
}

data "aws_route53_zone" "selected" {
  name         = var.hosted_zone
  private_zone = false
}

locals {
  serverIp   = var.create_lb ? aws_route53_record.aws_route53[0].fqdn : aws_instance.server.public_ip
  depends_on = [aws_instance.server]
}

resource "null_resource" "update_kubeconfig" {
  count      = var.no_of_server_nodes + var.etcd_only_nodes + var.etcd_cp_nodes + var.etcd_worker_nodes + var.cp_only_nodes + var.cp_worker_nodes
  depends_on = [aws_instance.server, aws_instance.server2]

  provisioner "local-exec" {
    command    = "scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i ${var.access_key} ${var.aws_user}@${count.index == 0 ? aws_instance.server.public_ip : aws_instance.server2[count.index - 1].public_ip}:/tmp/.control-plane /tmp/${var.resource_name}_control_plane_${count.index}"
    on_failure = continue
  }
  provisioner "local-exec" {
    command    = "test -f /tmp/${var.resource_name}_control_plane_${count.index} && sed s/127.0.0.1/\"${count.index == 0 ? local.serverIp : aws_instance.server2[count.index - 1].public_ip}\"/g /tmp/${var.resource_name}_config >/tmp/${var.resource_name}_kubeconfig"
    on_failure = continue
  }
}

resource "null_resource" "store_fqdn" {
  provisioner "local-exec" {
    command = "echo \"${var.create_lb ? aws_route53_record.aws_route53[0].fqdn : aws_instance.server.public_ip}\" >/tmp/${var.resource_name}_fixed_reg_addr"
  }
  depends_on = [aws_instance.server]
}