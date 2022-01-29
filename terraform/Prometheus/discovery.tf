resource "aws_service_discovery_private_dns_namespace" "prometheus" {
  name        = var.name
  description = "prometheus-stack"
  vpc         = "vpc-0e8d6207813fabfc4"
}