resource "aws_security_group" "worker_group_mgmt_one" {
  name_prefix = "accumulate-worker-group-mgmt-one"
  vpc_id = "vpc-0e8d6207813fabfc4"


ingress {
    from_port = 22
    to_port = 22
    protocol = "tcp"
 }
}