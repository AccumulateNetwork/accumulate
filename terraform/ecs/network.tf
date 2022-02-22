resource "aws_vpc" "dev_vpc" {
    cidr_block           = "10.0.0.0/16"
    enable_dns_support   = true #gives you an internal domain name
    enable_dns_hostnames = true #gives you an internal host name
    #enable_nat_gateway   = true
    #single_nat_gateway   = true
    enable_classiclink   = false
    assign_generated_ipv6_cidr_block = true 
    tags = {
    Name = "accumulate-dev"
  }
}

resource "aws_subnet" "dev_pubsub_a" {
    vpc_id                          = "${aws_vpc.dev_vpc.id}"
    cidr_block                      = "10.0.5.0/24"
    map_public_ip_on_launch         = true //it makes this a public subnet
    availability_zone               = "us-east-1a"   
    ipv6_cidr_block                 =  "${cidrsubnet(aws_vpc.dev_vpc.ipv6_cidr_block, 8, 1)}"
    assign_ipv6_address_on_creation = true
    tags = {
    Name = "accumulate-dev-public-a"
  }
}

resource "aws_subnet" "dev_pubsub_b" {
    vpc_id                          = "${aws_vpc.dev_vpc.id}"
    cidr_block                      = "10.0.6.0/24"
    map_public_ip_on_launch         = true //it makes this a public subnet
    availability_zone               = "us-east-1b" 
    ipv6_cidr_block                 = "${cidrsubnet(aws_vpc.dev_vpc.ipv6_cidr_block, 8, 2)}"
    assign_ipv6_address_on_creation = true  
    tags = {
    Name = "accumulate-dev-public-b"
  }
}

resource "aws_internet_gateway" "dev_igw" {
    vpc_id = "${aws_vpc.dev_vpc.id}"
}

resource "aws_route_table" "dev_public_crt" {
    vpc_id         = "${aws_vpc.dev_vpc.id}"
    tags = {
    Name = "accumulate-dev-public"
  }
    
    route {
        cidr_block = "0.0.0.0/0"
        gateway_id = "${aws_internet_gateway.dev_igw.id}" 
    }
    
    route {
        ipv6_cidr_block = "::/0"
        gateway_id      = "${aws_internet_gateway.dev_igw.id}"
    }
}

resource "aws_route_table_association" "dev_crta_public_subnet_a" {
    subnet_id      = "${aws_subnet.dev_pubsub_a.id}"
    route_table_id = "${aws_route_table.dev_public_crt.id}"
}

resource "aws_route_table_association" "dev_crta_public_subnet_b" {
    subnet_id      = "${aws_subnet.dev_pubsub_b.id}"
    route_table_id = "${aws_route_table.dev_public_crt.id}"
}

resource "aws_subnet" "dev_private_a" {
    vpc_id                  = "${aws_vpc.dev_vpc.id}"
    cidr_block              = "10.0.7.0/24"
    map_public_ip_on_launch = false  //it makes this a private subnet
    availability_zone       = "us-east-1a" 
    ipv6_cidr_block         =  "${cidrsubnet(aws_vpc.dev_vpc.ipv6_cidr_block, 8, 3)}"
    assign_ipv6_address_on_creation = true
    tags = {
    Name = "accumulate-dev-private-a"
  }  
}

resource "aws_subnet" "dev_private_b" {
    vpc_id                  = "${aws_vpc.dev_vpc.id}"
    cidr_block              = "10.0.8.0/24"
    map_public_ip_on_launch = false  //it makes this a private subnet
    availability_zone       = "us-east-1b" 
    ipv6_cidr_block                 =  "${cidrsubnet(aws_vpc.dev_vpc.ipv6_cidr_block, 8, 4)}"
    assign_ipv6_address_on_creation = true  
    tags = {
    Name = "accumulate-dev-private-b"
  }
}


resource "aws_route_table" "dev_private_crt" {
    vpc_id         = "${aws_vpc.dev_vpc.id}"
    tags = {
    Name = "accumulate-dev-private"
  }
}

resource "aws_route_table_association" "dev_crta_private_subnet_a" {
    subnet_id      = "${aws_subnet.dev_private_a.id}"
    route_table_id = "${aws_route_table.dev_private_crt.id}"
}



resource "aws_route_table_association" "dev_crta_private_subnet_b" {
    subnet_id      = "${aws_subnet.dev_private_b.id}"
    route_table_id = "${aws_route_table.dev_private_crt.id}"
}