resource "aws_efs_file_system" "devnet" {
  creation_token   = "efs2"
  performance_mode = "generalPurpose"
  throughput_mode  = "bursting"
  encrypted        = "true"
  tags = {
    Name = "accumulate-devnet"
  }
}

resource "aws_efs_access_point" "devnet" {
  file_system_id = aws_efs_file_system.devnet.id

  tags = {
    Name = "accumulate-devnet"
  }

  posix_user {
    gid = 1000
    uid = 1000
  }
}

resource "aws_efs_mount_target" "mount" {
  count           = length(data.aws_availability_zones.zones.names)
  file_system_id  = aws_efs_file_system.devnet.id
  subnet_id       = aws_subnet.subnet[count.index].id
  security_groups = [aws_security_group.efs.id]
}

resource "aws_security_group" "efs" {
  vpc_id            = aws_vpc.vpc.id
  name              = "accumulate-devnet-efs"

  ingress {
    from_port   = 111
    to_port     = 111
    protocol    = "tcp"
    security_groups = [
      aws_security_group.nodes.id,
    ]
  }

  ingress {
    from_port   = 2049
    to_port     = 2049
    protocol    = "tcp"
    security_groups = [
      aws_security_group.nodes.id,
    ]
  }
}
