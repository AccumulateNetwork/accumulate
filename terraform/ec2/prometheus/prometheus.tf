resource "aws_instance" "prometheus" {
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
  instance = aws_instance.prometheus.id
tags= {
    Name = "accumulate-prometheus-elastic-ip"
  }
}
