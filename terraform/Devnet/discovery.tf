resource "aws_service_discovery_private_dns_namespace" "devnet" {
  name        = "accumulate-devnet"
  description = "devnet-stack"
  vpc         = aws_vpc.vpc.id
}