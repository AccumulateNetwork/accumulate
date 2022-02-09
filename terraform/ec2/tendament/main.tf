resource "aws_security_group" "ec2_security_grp" {
  name        = var.security_group
  description = "security group for tendament"

  ingress {
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port   = 26656
    to_port     = 26666
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

 ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

 # outbound from jenkis server
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags= {
    Name = var.security_group
  }
}

resource "aws_instance" "myFirstInstance" {
  ami             = var.ami_id
  key_name        = var.key_name
  instance_type   = var.instance_type
#   security_groups = ["${aws_security_group.ec2_security_grp.id}"]
  security_groups = [var.security_group]
  tags= {
    Name = var.tag_name
  }
}

# Create Elastic IP address
resource "aws_eip" "myElasticIP" {
  vpc      = true
  instance = aws_instance.myFirstInstance.id
tags= {
    Name = "accumulate-tendament-elastic-ip"
  }
}