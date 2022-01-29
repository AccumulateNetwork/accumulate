resource "aws_alb" "application_load_balancer" {
  name               = var.name # Name of load balancer
  load_balancer_type = "application"
  subnets            = ["subnet-02c15750cd0264806","subnet-03c42789aa7599f12"]
  security_groups    = ["${aws_security_group.alb.id}"]
 }

resource "aws_alb_target_group" "prometheus" {
    name                 = var.name
    port                 = "90"
    protocol             = "HTTP"
    vpc_id               = "vpc-0e8d6207813fabfc4"
    deregistration_delay = 5
    target_type          = "ip"
    depends_on           = [aws_alb.application_load_balancer]

    lifecycle {
        create_before_destroy = true
    }

    health_check {
        healthy_threshold   = "2"
        unhealthy_threshold = "2"
        interval            = "30"
        matcher             = "200"
        path                = "/graph"
        port                = "traffic-port"
        protocol            = "HTTP"
        timeout             = "3"
    }

}


resource "aws_alb_listener" "prometheus" {
   load_balancer_arn = "${aws_alb.application_load_balancer.arn}"
    port              = "90"
    protocol          = "HTTP"

    default_action {
        target_group_arn = "${aws_alb_target_group.prometheus.arn}"
        type             = "forward"
    }
}