resource "aws_efs_file_system" "devnet" {
   creation_token   = "efs2"
   performance_mode = "generalPurpose"
   throughput_mode  = "bursting"
   encrypted        = "true"
   tags = {
    Name = "accumulate-devnet"
  }
}

output "efs" {
  value = aws_efs_file_system.devnet.dns_name
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

  root_directory {
    path = "/"
    creation_info {
      owner_gid = 1000
      owner_uid = 1000
      permissions = 755
    }
  }
}

resource "aws_efs_mount_target" "devnet" {
  count           = length(data.aws_availability_zones.available.names)
  file_system_id  = aws_efs_file_system.devnet.id
  subnet_id       = aws_subnet.subnet[count.index].id
  security_groups = [
    aws_security_group.efs_dev.id,
    aws_security_group.dev_tools.id,
    aws_security_group.devnet.id,
    aws_security_group.ec2.id,
  ]
}
