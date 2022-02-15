resource "aws_security_group" "efs_security_group" {
  vpc_id        = "vpc-0e8d6207813fabfc4"
  name          = "accumulate-testnet-efs"
  ingress {
    from_port   = 2049
    to_port     = 2049
    protocol    = "tcp"
    security_groups = ["${aws_security_group.tool_service.id}", "${aws_security_group.testnet.id}"]
  }


  egress {
    from_port       = 0
    to_port         = 0
    protocol        = "-1"
    security_groups = ["${aws_security_group.tool_service.id}", "${aws_security_group.testnet.id}"]
  }
}


resource "aws_security_group" "tool_service" {
  vpc_id        = "vpc-0e8d6207813fabfc4"
  name          = "accumulate-testnet-tools"
 
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
      from_port = 0
      to_port = 0
      protocol = "-1"
      ipv6_cidr_blocks = ["::/0"]
    }
    
}

resource "aws_security_group_rule" "tool_allow_testnet" {
    type                     = "ingress"
    from_port                = 0
    to_port                  = 26660
    protocol                 = "tcp"
 #   ipv6_cidr_blocks         = ["::/0"]
    security_group_id        = "${aws_security_group.tool_service.id}"
    source_security_group_id = "${aws_security_group.testnet.id}"
}

resource "aws_security_group_rule" "allow_efs" {
    type                     = "ingress"
    from_port                = 2049
    to_port                  = 2049
    protocol                 = "tcp"
    security_group_id        = "${aws_security_group.tool_service.id}"
    source_security_group_id = "${aws_security_group.efs_security_group.id}"
}

resource "aws_security_group_rule" "ingress_docker_ports" {
  type              = "ingress"
  from_port         = 26656
  to_port           = 26666
  protocol          = "tcp"
  ipv6_cidr_blocks  = ["::/0"]
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = "${aws_security_group.tool_service.id}"
}

resource "aws_security_group_rule" "allow_docker" {
  type              = "ingress"
  from_port         = 8080
  to_port           = 8080
  protocol          = "tcp"
  ipv6_cidr_blocks  = ["::/0"]
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = "${aws_security_group.tool_service.id}"
}


resource "aws_security_group" "load_balancer_security_group" {
    vpc_id      = "vpc-0e8d6207813fabfc4"
    name        = "accumulate-testnet-alb"
  ingress {
    from_port   = 0
    to_port     = 26660
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"] # Allowing traffic in from all sources
  }


  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}


resource "aws_security_group" "testnet" {
  vpc_id            = "vpc-0e8d6207813fabfc4"
  name              = "accumulate-testnet" 
  ingress {
    from_port       = 0
    to_port         = 26660
    protocol        = "tcp"
    ipv6_cidr_blocks = ["::/0"]
    # allowing traffic in from the load balancer security group and efs mount target security group
    security_groups = ["${aws_security_group.load_balancer_security_group.id}","${aws_security_group.tool_service.id}"]
  }


  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
      from_port = 0
      to_port = 0
      protocol = "-1"
      ipv6_cidr_blocks = ["::/0"]
    }

}


resource "aws_security_group_rule" "allow_efs_2" {
    type                     = "ingress"
    from_port                = 2049
    to_port                  = 2049
    protocol                 = "tcp"
    security_group_id        = "${aws_security_group.testnet.id}"
    source_security_group_id = "${aws_security_group.efs_security_group.id}"
}

resource "aws_security_group_rule" "ingress_docker_ports_2" {
  type              = "ingress"
  from_port         = 26656
  to_port           = 26666
  protocol          = "tcp"
  ipv6_cidr_blocks  = ["::/0"]
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = "${aws_security_group.testnet.id}"
}

resource "aws_security_group_rule" "allow_docker_2" {
  type              = "ingress"
  from_port         = 8080
  to_port           = 8080
  protocol          = "tcp"
  ipv6_cidr_blocks  = ["::/0"]
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = "${aws_security_group.testnet.id}"
}