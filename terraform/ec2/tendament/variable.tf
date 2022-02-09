variable "aws_region" {
       description = "The AWS region to create things in." 
       default     = "us-east-1" 
}

variable "key_name" { 
    description = " SSH keys to connect to ec2 instance" 
    default     =  "tendament" 
}

variable "instance_type" { 
    description = "instance type for ec2" 
    default     =  "m6g.large" 
}

variable "security_group" { 
    description = "Name of security group" 
    default     = "accumulate-tenament-ec2-group" 
}

variable "tag_name" { 
    description = "Tag Name of for Ec2 instance" 
    default     = "accumulate-tendament-instance" 
} 
variable "ami_id" { 
    description = "AMI for Ubuntu Ec2 instance" 
    default     = "ami-0b49a4a6e8e22fa16" 
}