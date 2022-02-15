output "testnet_sg" {
  value = aws_security_group.testnet.id
}

output "efs" {
  value = aws_efs_file_system.testnet.id
}

output "efs_ac" {
  value = aws_efs_access_point.testnet.id
}

output "role" {
  value = aws_iam_role.ecsTaskExecutionRole.arn
}

output "lb" {
  value = aws_security_group.load_balancer_security_group.id
}