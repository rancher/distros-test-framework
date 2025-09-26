resource "aws_lb_target_group" "aws_tg_6443" {
  port     = 6443
  protocol = "TCP"
  vpc_id   = var.vpc_id
  name     = "${var.resource_name}-${local.resource_tag}-tg-6443"
  count    = var.create_lb && var.product == "rke2" ? 1 : 0
}

resource "aws_lb_target_group" "aws_tg_9345" {
  port     = 9345
  protocol = "TCP"
  vpc_id   = var.vpc_id
  name     = "${var.resource_name}-${local.resource_tag}-tg-9345"
  count    = var.create_lb ? 1 : 0
}

resource "aws_lb_target_group" "aws_tg_80" {
  port     = 80
  protocol = "TCP"
  vpc_id   = var.vpc_id
  name     = "${var.resource_name}-${local.resource_tag}-tg-80"
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
  name     = "${var.resource_name}-${local.resource_tag}-tg-443"
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
  target_id        = aws_instance.master[count.index].id
  port             = 6443
  depends_on       = [aws_instance.master]
  count            = var.create_lb ? 1 : 0
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_6443_2" {
  target_group_arn = aws_lb_target_group.aws_tg_6443[0].arn
  count            = var.create_lb ? length(aws_instance.master2-ha) : 0
  target_id        = aws_instance.master[count.index].id
  depends_on       = [aws_instance.master2-ha]
  port             = 6443
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_9345" {
  target_group_arn = aws_lb_target_group.aws_tg_9345[0].arn
  target_id        = aws_instance.master.id
  port             = 9345
  depends_on       = [aws_instance.master]
  count            = var.create_lb ? 1 : 0
}
resource "aws_lb_target_group_attachment" "aws_tg_attachment_9345_2" {
  target_group_arn = aws_lb_target_group.aws_tg_9345[0].arn
  count            = var.create_lb ? length(aws_instance.master2-ha) : 0
  target_id        = aws_instance.master2-ha[count.index].id
  depends_on       = [aws_instance.master2-ha]
  port             = 9345
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_80" {
  target_group_arn = aws_lb_target_group.aws_tg_80[0].arn
  target_id        = aws_instance.master.id
  port             = 80
  depends_on       = [aws_instance.master]
  count            = var.create_lb ? 1 : 0
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_80_2" {
  target_group_arn = aws_lb_target_group.aws_tg_80[0].arn
  count            = var.create_lb ? length(aws_instance.master2-ha) : 0
  target_id        = aws_instance.master2-ha[count.index].id
  port             = 80
  depends_on       = [aws_instance.master]
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_443" {
  target_group_arn = aws_lb_target_group.aws_tg_443[0].arn
  target_id        = aws_instance.master.id
  port             = 443
  depends_on       = [aws_instance.master]
  count            = var.create_lb ? 1 : 0
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_443_2" {
  target_group_arn = aws_lb_target_group.aws_tg_443[0].arn
  count            = var.create_lb ? length(aws_instance.master2-ha) : 0
  target_id        = aws_instance.master2-ha[count.index].id
  port             = 443
  depends_on       = [aws_instance.master]
}

resource "aws_lb" "aws_nlb" {
  internal           = false
  load_balancer_type = "network"
  subnets            = [var.subnets]
  name               = "${var.resource_name}-${local.resource_tag}-nlb"
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
  name    = "${var.resource_name}-${local.resource_tag}-r53"
  type    = "CNAME"
  ttl     = "300"
  records = [aws_lb.aws_nlb[0].dns_name]
  count   = var.create_lb ? 1 : 0
}

data "aws_route53_zone" "selected" {
  name         = var.hosted_zone
  private_zone = false
}

# ################ K3S ###################

# resource "aws_lb_target_group" "aws_tg_80" {
#   count              = var.create_lb ? 1 : 0
#   port               = 80
#   protocol           = "TCP"
#   vpc_id             = var.vpc_id
#   name               = "${var.resource_name}-${local.resource_tag}-tg-80"
#   health_check {
#         protocol            = "HTTP"
#         port                = "traffic-port"
#         path                = "/ping"
#         interval            = 10
#         timeout             = 6
#         healthy_threshold   = 3
#         unhealthy_threshold = 3
#         matcher             = "200-399"
#   }
# }

# resource "aws_lb_target_group_attachment" "aws_tg_attachment_80" {
#   count              = var.create_lb ? 1 : 0
#   depends_on         = [aws_instance.master[count.index]]
#   target_group_arn   = aws_lb_target_group.aws_tg_80[0].arn
#   target_id          = aws_instance.master[count.index].id
#   port               = 80
# }

# resource "aws_lb_target_group_attachment" "aws_tg_attachment_80_2" {
#   count              = var.create_lb ? length(aws_instance.master) - 1 : 0
#   depends_on         = [aws_instance.master[count.index]]
#   target_id          = aws_instance.master[count.index + 1].id
#   target_group_arn   = aws_lb_target_group.aws_tg_80[0].arn
#   port               = 80

# }

# resource "aws_lb_target_group" "aws_tg_443" {
#   count              = var.create_lb ? 1 : 0
#   port               = 443
#   protocol           = "TCP"
#   vpc_id             = var.vpc_id
#   name               = "${var.resource_name}-${local.resource_tag}-tg-443"
#   health_check {
#         protocol            = "HTTP"
#         port                = 80
#         path                = "/ping"
#         interval            = 10
#         timeout             = 6
#         healthy_threshold   = 3
#         unhealthy_threshold = 3
#         matcher             = "200-399"
#   }
# }

# resource "aws_lb_target_group_attachment" "aws_tg_attachment_443" {
#   count              = var.create_lb ? 1 : 0
#   depends_on         = [aws_instance.master[0]]
#   target_group_arn   = aws_lb_target_group.aws_tg_443[0].arn
#   target_id          = aws_instance.master[0].id
#   port               = 443
# }

# resource "aws_lb_target_group_attachment" "aws_tg_attachment_443_2" {
#   count              = var.create_lb ? length(aws_instance.master) - 1 : 0
#   depends_on         = [aws_instance.master[0]]
#   target_group_arn   = aws_lb_target_group.aws_tg_443[0].arn
#   target_id          = aws_instance.master[count.index + 1].id
#   port               = 443
# }

# resource "aws_lb_target_group" "aws_tg_6443" {
#   count              = var.create_lb ? 1 : 0
#   port               = 6443
#   protocol           = "TCP"
#   vpc_id             = var.vpc_id
#   name               = "${var.resource_name}-${local.resource_tag}-tg-6443"
# }

# resource "aws_lb_target_group_attachment" "aws_tg_attachment_6443" {
#   count              = var.create_lb ? 1 : 0
#   depends_on         = [aws_instance.master[0]]
#   target_group_arn   = aws_lb_target_group.aws_tg_6443[0].arn
#   target_id          = aws_instance.master[0].id
#   port               = 6443
# }

# resource "aws_lb_target_group_attachment" "aws_tg_attachment_6443_2" {
#   count              = var.create_lb ? length(aws_instance.master) - 1 : 0
#   depends_on         = [aws_instance.master[0]]
#   target_group_arn   = aws_lb_target_group.aws_tg_6443[0].arn
#   target_id          = aws_instance.master[count.index + 1].id
#   port               = 6443
# }

# resource "aws_lb" "aws_nlb" {
#   count              = var.create_lb ? 1 : 0
#   internal           = false
#   load_balancer_type = "network"
#   subnets            = [var.subnets]
#   name               = "${var.resource_name}-${local.resource_tag}-nlb"
# }

# resource "aws_lb_listener" "aws_nlb_listener_80" {
#   count              = var.create_lb ? 1 : 0
#   load_balancer_arn  = aws_lb.aws_nlb[0].arn
#   port               = "80"
#   protocol           = "TCP"
#   default_action {
#     type             = "forward"
#     target_group_arn = aws_lb_target_group.aws_tg_80[0].arn
#   }
# }

# resource "aws_lb_listener" "aws_nlb_listener_443" {
#   count              = var.create_lb ? 1 : 0
#   load_balancer_arn  = aws_lb.aws_nlb[0].arn
#   port               = "443"
#   protocol           = "TCP"
#   default_action {
#     type             = "forward"
#     target_group_arn = aws_lb_target_group.aws_tg_443[0].arn
#   }
# }

# resource "aws_lb_listener" "aws_nlb_listener_6443" {
#   count              = var.create_lb ? 1 : 0
#   load_balancer_arn  = aws_lb.aws_nlb[0].arn
#   port               = "6443"
#   protocol           = "TCP"
#   default_action {
#     type             = "forward"
#     target_group_arn = aws_lb_target_group.aws_tg_6443[0].arn
#   }
# }

# resource "aws_route53_record" "aws_route53" {
#   count              = var.create_lb ? 1 : 0
#   depends_on         = [aws_lb_listener.aws_nlb_listener_6443]
#   zone_id            = data.aws_route53_zone.selected.zone_id
#   name               = "${var.resource_name}-${local.resource_tag}-r53"
#   type               = "CNAME"
#   ttl                = "300"
#   records            = [aws_lb.aws_nlb[0].dns_name]
# }

# data "aws_route53_zone" "selected" {
#   name               = var.hosted_zone
#   private_zone       = false
# }
