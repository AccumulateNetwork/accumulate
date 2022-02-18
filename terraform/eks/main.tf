module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "18.7.2"
  cluster_name = var.cluster_name
  cluster_ip_family = "ipv4"
  cluster_security_group_name = "accumulate-testnet-sg"
  cluster_version = "1.21"
  cluster_timeouts = {
      create = "1h"
      delete = "1h"
      }
  cluster_additional_security_group_ids = [aws_security_group.worker_group_mgmt_one.id]
  node_security_group_name = "accumulate-node-sg"
  vpc_id = "vpc-0e8d6207813fabfc4"
  cluster_endpoint_private_access = true
  subnet_ids = ["subnet-02c15750cd0264806","subnet-03c42789aa7599f12"]

#   self_managed_node_groups = [
#       {
#           name                      = "worker-group-1"
#           instance_type             = "t2.large"
#           additional_userdata       = "echo foo bar"
#           asg_desired_capacity      = 1
#           additional_security_group_ids = [aws_security_group.worker_group_mgmt_one.id]
#       },
#   ]
}

data "aws_eks_cluster" "cluster" {
  name = module.eks.cluster_id
}

data "aws_eks_cluster_auth" "cluster" {
  name = module.eks.cluster_id
}
