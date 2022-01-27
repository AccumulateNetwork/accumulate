resource "aws_security_group" "alb_security_group" {
    vpc_id      = "${aws_vpc.dev_vpc.id}"
    name        = "accumulate-devnet-alb"
  ingress {
    from_port   = 26660
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

resource "aws_security_group" "dev_tools" {
  vpc_id        = "${aws_vpc.dev_vpc.id}"
  name          = "accumulate-devnet-tools"

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

resource "aws_security_group_rule" "tool_allow_devnet" {
    type                     = "ingress"
    from_port                = 0
    to_port                  = 26660
    protocol                 = "tcp"
    security_group_id        = "${aws_security_group.dev_tools.id}"
    source_security_group_id = "${aws_security_group.devnet.id}"
}

resource "aws_security_group_rule" "devtools_allow_efs" {
    type                     = "ingress"
    from_port                = 2049
    to_port                  = 2049
    protocol                 = "tcp"
    security_group_id        = "${aws_security_group.dev_tools.id}"
    source_security_group_id = "${aws_security_group.efs_dev.id}"
}

resource "aws_security_group_rule" "ingress_docker_devtools" {
  type              = "ingress"
  from_port         = 26656
  to_port           = 26666
  protocol          = "tcp"
  ipv6_cidr_blocks  = ["::/0"]
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = "${aws_security_group.dev_tools.id}"
}

resource "aws_security_group_rule" "allow_docker" {
  type              = "ingress"
  from_port         = 8080
  to_port           = 8080
  protocol          = "tcp"
  ipv6_cidr_blocks  = ["::/0"]
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = "${aws_security_group.dev_tools.id}"
}

resource "aws_security_group" "devnet" {
  vpc_id            = "${aws_vpc.dev_vpc.id}"
  name              = "accumulate-devnet-nodes"
  ingress {
    from_port       = 0
    to_port         = 26660
    protocol        = "tcp"
    ipv6_cidr_blocks = ["::/0"]
    # allowing traffic in from the load balancer security group and efs mount target security group
    security_groups = ["${aws_security_group.alb_security_group.id}",
                         "${aws_security_group.dev_tools.id}"]
  }


  egress {
    from_port       = 0
    to_port         = 0
    protocol        = "-1"
    cidr_blocks     = ["0.0.0.0/0"]
  }

  egress {
      from_port        = 0
      to_port          = 0
      protocol         = "-1"
      ipv6_cidr_blocks = ["::/0"]
    }
}

resource "aws_security_group_rule" "allow_efs_dev" {
    type                     = "ingress"
    from_port                = 2049
    to_port                  = 2049
    protocol                 = "tcp"
    security_group_id        = "${aws_security_group.devnet.id}"
    source_security_group_id = "${aws_security_group.efs_dev.id}"
}

resource "aws_security_group_rule" "ingress_docker_devnet" {
  type              = "ingress"
  from_port         = 26656
  to_port           = 26666
  protocol          = "tcp"
  ipv6_cidr_blocks  = ["::/0"]
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = "${aws_security_group.devnet.id}"
}

resource "aws_security_group_rule" "allow_docker_2" {
  type              = "ingress"
  from_port         = 8080
  to_port           = 8080
  protocol          = "tcp"
  ipv6_cidr_blocks  = ["::/0"]
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = "${aws_security_group.devnet.id}"
}

resource "aws_security_group" "efs_dev" {
  vpc_id            = "${aws_vpc.dev_vpc.id}"
  name              = "accumulate-devnet-efs"

  ingress {
    from_port   = 2049
    to_port     = 2049
    protocol    = "tcp"
    security_groups = ["${aws_security_group.dev_tools.id}", "${aws_security_group.devnet.id}"]
  }

  egress {
    from_port       = 0
    to_port         = 0
    protocol        = "-1"
    security_groups = ["${aws_security_group.devnet.id}", "${aws_security_group.dev_tools.id}"]
  }
}


