resource "aws_efs_file_system" "prometheus" {
   creation_token   = "prometheus"
   performance_mode = "generalPurpose"
   throughput_mode  = "bursting"
   encrypted        = "true"
   tags = {
    Name = var.name
  }
}



resource "aws_efs_access_point" "prometheus" {
  file_system_id = aws_efs_file_system.prometheus.id
  tags = {
    Name = var.name
  }
  posix_user {
    gid = 1000
    uid = 1000
  }

  root_directory {
    path = "/etc/prometheus"
    creation_info {
      owner_gid = 1000
      owner_uid = 1000
      permissions = 755
    }
  }
}

resource "aws_efs_mount_target" "main" {
  file_system_id  = aws_efs_file_system.prometheus.id
  subnet_id       = "subnet-0813c2cde2e5d9399"
  security_groups = ["${aws_security_group.efs_security_group.id}",
  "${aws_security_group.prometheus.id}"
  ]
}


resource "aws_efs_mount_target" "main_1" {
  file_system_id  = aws_efs_file_system.prometheus.id
  subnet_id       = "subnet-031a74b80f3bbf3d2"
  security_groups = ["${aws_security_group.efs_security_group.id}",
  "${aws_security_group.prometheus.id}" 
  ]
}
