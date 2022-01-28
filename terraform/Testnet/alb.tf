resource "aws_alb" "application_load_balancer" {
  name               = "accumulate-testnet" # Name of load balancer
  load_balancer_type = "application"
  subnets            = ["subnet-02c15750cd0264806","subnet-03c42789aa7599f12"]
  security_groups    = ["${aws_security_group.load_balancer_security_group.id}"]
 }


resource "aws_alb_target_group" "target_group" {
  name                        = "accumulate-target-group"
  port                        = "26660"
  protocol                    = "HTTP"
  target_type                 = "ip"
  vpc_id                      = "vpc-0e8d6207813fabfc4" # Reference the VPC  
  depends_on                  = [aws_alb.application_load_balancer]
  deregistration_delay        = 50
  
  lifecycle {
        create_before_destroy = true
    }
  
  health_check {
    healthy_threshold         = 2
    unhealthy_threshold       = 10
    interval                  = 300
    matcher                   = "200"
    path                      = "/"
    protocol                  = "HTTP"
    port                      = "traffic-port"
    
  }

}

resource "aws_alb_listener" "listener" {
  load_balancer_arn    = "${aws_alb.application_load_balancer.id}" # Reference our load balancer
  port                 = "26660"
  protocol             = "HTTP"
  default_action {
    type               = "forward"
    target_group_arn   = "${aws_alb_target_group.target_group.arn}" # Reference our target group
  }
      depends_on       = [aws_alb.application_load_balancer]
}