variable "aws_region" {
       description = "The AWS region to create things in." 
       default     = "us-east-1" 
}

variable "key_name" { 
    description = " SSH keys to connect to ec2 instance" 
    default     =  "stephen1" 
}

variable "instance_type" { 
    description = "instance type for ec2" 
    default     =  "t2.large" 
}

variable "security_group" { 
    description = "Name of security group" 
    default     = "accumulate-prometheus-ec2-group" 
}

variable "tag_name1" { 
    description = "Tag Name of for Ec2 instance" 
    default     = "accumulate-prometheus-instance" 
}  

variable "ami_id" { 
    description = "AMI for Ubuntu Ec2 instance" 
    default     = "ami-04505e74c0741db8d" 
}

