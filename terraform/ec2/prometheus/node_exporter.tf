resource "aws_instance" "node_exporter" {
  ami             = var.ami_id
  key_name        = var.key_name
  instance_type   = var.instance_type
#   security_groups = ["${aws_security_group.ec2_security_grp.id}"]
  security_groups = [var.security_group]
  tags= {
    Name = var.tag_name2
  }
}

# Create Elastic IP address
resource "aws_eip" "myElasticIP2" {
  vpc      = true
  instance = aws_instance.node_exporter.id
tags= {
    Name = "accumulate-node-exporter-elastic-ip"
  }
}