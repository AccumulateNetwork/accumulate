data "aws_acm_certificate" "issued" {
  domain   = "devnet.accumulatenetwork.io"
  statuses = ["ISSUED"]
}

resource "aws_alb" "dev_alb" {
  name               = "accumulate-devnet-alb" # Name of load balancer
  load_balancer_type = "application"
  subnets            = aws_subnet.subnet.*.id
  security_groups    = ["${aws_security_group.alb_security_group.id}"]
 }
 
output "alb_hostname" {
  value = aws_alb.dev_alb.dns_name
}

resource "aws_alb_target_group" "dev_target" {
  name        = "accumulate-dev-target"
  port        = "26660"
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = aws_vpc.vpc.id
  depends_on  = [aws_alb.dev_alb]
  deregistration_delay = 50

  lifecycle {
    create_before_destroy = true
  }

  health_check {
    healthy_threshold   = 2
    unhealthy_threshold = 10
    interval            = 300
    matcher             = "200"
    path                = "/status"
    protocol            = "HTTP"
    port                = "traffic-port"
  }
}

resource "aws_alb_listener" "dev_listener" {
  load_balancer_arn = "${aws_alb.dev_alb.id}" # Reference our load balancer
  port              = "80"
  protocol          = "HTTP"
  default_action {
    type             = "forward"
    target_group_arn = "${aws_alb_target_group.dev_target.arn}" # Reference our target group
  }
  depends_on       = [aws_alb.dev_alb]
}

resource "aws_alb_listener" "dev_listener_https" {
  load_balancer_arn = "${aws_alb.dev_alb.id}" # Reference our load balancer
  certificate_arn   = data.aws_acm_certificate.issued.arn
  port              = "443"
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-2016-08"

  default_action {
    type             = "forward"
    target_group_arn = "${aws_alb_target_group.dev_target.arn}" # Reference our target group
  }
  depends_on       = [aws_alb.dev_alb]
}