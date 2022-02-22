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

  root_directory {
    path = "/mnt/efs/node"
    creation_info {
      owner_gid = 1000
      owner_uid = 1000
      permissions = 755
    }
  }
}

resource "aws_efs_mount_target" "dev" {
  file_system_id  = aws_efs_file_system.devnet.id
  subnet_id       = aws_subnet.dev_private_a.id 
  security_groups = ["${aws_security_group.efs_dev.id}",
      "${aws_security_group.dev_tools.id}",
      "${aws_security_group.devnet.id}"
    ]

}

resource "aws_efs_mount_target" "dev_2" {
  file_system_id  = aws_efs_file_system.devnet.id
  subnet_id       = aws_subnet.dev_private_b.id
  security_groups = ["${aws_security_group.efs_dev.id}",
      "${aws_security_group.dev_tools.id}",
      "${aws_security_group.devnet.id}"
  ]

}