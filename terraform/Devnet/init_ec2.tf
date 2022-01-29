resource "aws_instance" "init" {
  ami = "ami-0b6705f88b1f688c1"
  instance_type = "t4g.micro"
  key_name = "ethan2"
  security_groups  = [aws_security_group.ec2.id]
  subnet_id = aws_subnet.subnet[0].id

  tags = {
    Name = "accumulate-devnet-init"
  }

  lifecycle {
    ignore_changes = [security_groups]
  }
}

output "init_ip" {
  value = aws_instance.init.public_ip
}

resource "aws_security_group" "ec2" {
  vpc_id            = aws_vpc.vpc.id
  name              = "accumulate-devnet-ec2"

  # SSH
  ingress {
    description      = ""
    from_port        = 22
    to_port          = 22
    protocol         = "tcp"
    ipv6_cidr_blocks = ["::/0"]
    cidr_blocks      = ["0.0.0.0/0"]
  }
}