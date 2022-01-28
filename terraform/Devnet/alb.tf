resource "aws_alb" "dev_alb" {
  name               = "accumulate-devnet-alb" # Name of load balancer
  load_balancer_type = "application"
  subnets            = ["${aws_subnet.dev_pubsub_a.id}",
                         "${aws_subnet.dev_pubsub_b.id}"]
  security_groups    = ["${aws_security_group.alb_security_group.id}"]
 }


resource "aws_alb_target_group" "dev_target" {
  name        = "accumulate-dev-target"
  port        = "26660"
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = "${aws_vpc.dev_vpc.id}"
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
  certificate_arn   = "arn:aws:acm:us-east-1:018508593216:certificate/d1a0d5d7-b237-455b-9757-27e85a945e9d"
  port              = "443"
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-2016-08"

  default_action {
    type             = "forward"
    target_group_arn = "${aws_alb_target_group.dev_target.arn}" # Reference our target group
  }
  depends_on       = [aws_alb.dev_alb]
}