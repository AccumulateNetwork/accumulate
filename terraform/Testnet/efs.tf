resource "aws_efs_file_system" "testnet" {
   creation_token   = "efs"
   performance_mode = "generalPurpose"
   throughput_mode  = "bursting"
   encrypted        = "true"
   tags = {
    Name = "accumulate-testnet"
  }
}



resource "aws_efs_access_point" "testnet" {
  file_system_id = aws_efs_file_system.testnet.id
  tags = {
    Name = "accumulate-testnet"
  }
  posix_user {
    gid = 1000
    uid = 1000
  }

  root_directory {
    path = "/mnt/efs/node"
    creation_info {
      owner_gid = 1000
      owner_uid = 1000
      permissions = 755
    }
  }
}

resource "aws_efs_mount_target" "main" {
  file_system_id  = aws_efs_file_system.testnet.id
  subnet_id       = "subnet-0813c2cde2e5d9399"
  security_groups = ["${aws_security_group.efs_security_group.id}","${aws_security_group.tool_service.id}",
  "${aws_security_group.testnet.id}"
  ]
}


resource "aws_efs_mount_target" "main_1" {
  file_system_id  = aws_efs_file_system.testnet.id
  subnet_id       = "subnet-031a74b80f3bbf3d2"
  security_groups = ["${aws_security_group.efs_security_group.id}","${aws_security_group.tool_service.id}",
  "${aws_security_group.testnet.id}"
  ]
}



