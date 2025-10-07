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
  count    = (var.product == "rke2" && var.create_lb) ? 1 : 0
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
  depends_on       = [aws_instance.master[0]]
  count            = var.create_lb ? 1 : 0
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_9345" {
  target_group_arn = aws_lb_target_group.aws_tg_9345[0].arn
  target_id        = aws_instance.master[0].id
  port             = 9345
  depends_on       = [aws_instance.master[0]]
  count            = (var.product == "rke2" && var.create_lb) ? 1 : 0
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_80" {
  target_group_arn = aws_lb_target_group.aws_tg_80[0].arn
  target_id        = aws_instance.master[0].id
  port             = 80
  depends_on       = [aws_instance.master[0]]
  count            = var.create_lb ? 1 : 0
}

resource "aws_lb_target_group_attachment" "aws_tg_attachment_443" {
  target_group_arn = aws_lb_target_group.aws_tg_443[0].arn
  target_id        = aws_instance.master[0].id
  port             = 443
  depends_on       = [aws_instance.master[0]]
  count            = var.create_lb ? 1 : 0
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
  count = (var.product == "rke2" && var.create_lb) ? 1 : 0
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
