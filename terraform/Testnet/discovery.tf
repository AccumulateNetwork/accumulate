resource "aws_service_discovery_private_dns_namespace" "simple-stack" {
  name        = "accumulate-testnet"
  description = "testnet-stack"
  vpc         = "vpc-0e8d6207813fabfc4"
}


