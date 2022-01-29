data "aws_availability_zones" "zones" {}

resource "aws_vpc" "vpc" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_support   = true
  enable_dns_hostnames = true
  enable_classiclink   = false
  assign_generated_ipv6_cidr_block = true

  tags = {
    Name = "accumulate-devnet"
  }
}

resource "aws_internet_gateway" "igw" {
  vpc_id = aws_vpc.vpc.id

  tags = {
    Name = "accumulate-devnet"
  }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.vpc.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.igw.id
  }

  route {
    ipv6_cidr_block = "::/0"
    gateway_id      = aws_internet_gateway.igw.id
  }

  tags = {
    Name = "accumulate-devnet"
  }
}

resource "aws_subnet" "subnet" {
  count                           = length(data.aws_availability_zones.zones.names)
  vpc_id                          = aws_vpc.vpc.id
  availability_zone               = data.aws_availability_zones.zones.names[count.index]
  cidr_block                      = cidrsubnet(aws_vpc.vpc.cidr_block, 8, count.index)
  ipv6_cidr_block                 = cidrsubnet(aws_vpc.vpc.ipv6_cidr_block, 8, count.index)
  map_public_ip_on_launch         = true
  assign_ipv6_address_on_creation = true

  tags = {
    Name = "accumulate-devnet-${count.index}"
  }
}

resource "aws_route_table_association" "subnet_public" {
  count          = length(data.aws_availability_zones.zones.names)
  subnet_id      = aws_subnet.subnet[count.index].id
  route_table_id = aws_route_table.public.id
}
