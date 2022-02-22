resource "aws_security_group" "alb_security_group" {
    name = "accumulate-alb-security-group"
    vpc_id      = "${aws_vpc.dev_vpc.id}"
  ingress {
    from_port   = 0
    to_port     = 80
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
  name = "accumulate-dev-tools"
  vpc_id            = "${aws_vpc.dev_vpc.id}"
  ingress {
    from_port       = 22
    to_port         = 22
    protocol        = "tcp"
    cidr_blocks     = ["0.0.0.0/0"]
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

resource "aws_security_group_rule" "tool_allow_devnet" {
    type                     = "ingress"
    from_port                = 9000
    to_port                  = 9000
    protocol                 = "tcp"
    #ipv6_cidr_blocks         = ["::/0"]
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
  to_port           = 26662
  protocol          = "tcp"
  ipv6_cidr_blocks  = ["::/0"]
  security_group_id = "${aws_security_group.dev_tools.id}"
}


resource "aws_security_group" "devnet" {
  name = "accumulate-devnet-service"
  vpc_id            = "${aws_vpc.dev_vpc.id}"
  ingress {
    from_port       = 26660
    to_port         = 26660
    protocol        = "tcp"
    # allowing traffic in from the load balancer security group and efs mount target security group
    security_groups = ["${aws_security_group.alb_security_group.id}",
                         "${aws_security_group.dev_tools.id}"]
  }

  ingress {
    from_port       = 22
    to_port         = 22
    protocol        = "tcp"
    cidr_blocks     = ["0.0.0.0/0"]
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
  to_port           = 26662
  protocol          = "tcp"
  ipv6_cidr_blocks  = ["::/0"]
  security_group_id = "${aws_security_group.devnet.id}"
}

resource "aws_security_group" "efs_dev" {
  name              = "accumulate-efs"
  vpc_id            = "${aws_vpc.dev_vpc.id}"

  egress {
    from_port       = 0
    to_port         = 0
    protocol        = "-1"
    security_groups = ["${aws_security_group.devnet.id}", "${aws_security_group.dev_tools.id}"]
  }
}

resource "aws_security_group_rule" "allow_devnet" {
    type                     = "ingress"
    from_port                = 2049
    to_port                  = 2049
    protocol                 = "tcp"
    security_group_id        = "${aws_security_group.efs_dev.id}"
    source_security_group_id = "${aws_security_group.devnet.id}"
}

resource "aws_security_group_rule" "allow_devtools" {
    type                     = "ingress"
    from_port                = 2049
    to_port                  = 2049
    protocol                 = "tcp"
    security_group_id        = "${aws_security_group.efs_dev.id}"
    source_security_group_id = "${aws_security_group.dev_tools.id}"
}

