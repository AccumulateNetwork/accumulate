resource "aws_security_group" "prometheus" {
  vpc_id      = "vpc-0e8d6207813fabfc4"
  name = var.name 
  ingress {
    from_port = 0
    to_port   = 0
    protocol  = "-1"
    security_groups = ["${aws_security_group.alb.id}"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group_rule" "allow_efs" {
    type                     = "ingress"
    from_port                = 2049
    to_port                  = 2049
    protocol                 = "tcp"
    security_group_id        = "${aws_security_group.prometheus.id}"
    source_security_group_id = "${aws_security_group.efs_security_group.id}"
}

resource "aws_security_group" "efs_security_group" {
  vpc_id        = "vpc-0e8d6207813fabfc4"
  name          = "accumulate-prometheus-efs"
  ingress {
    from_port   = 2049
    to_port     = 2049
    protocol    = "tcp"
    security_groups = ["${aws_security_group.prometheus.id}"]
  }


  egress {
    from_port       = 0
    to_port         = 0
    protocol        = "-1"
    security_groups = ["${aws_security_group.prometheus.id}"]
 }
}

resource "aws_security_group" "alb" {
  vpc_id = "vpc-0e8d6207813fabfc4"
  ingress {
    from_port   = 90
    to_port     = 90
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
