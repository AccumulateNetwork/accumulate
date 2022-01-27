variable "name" {
  type = string
  default = "devnet"
}

variable "image" {
  type = string
  default = "registry.gitlab.com/accumulatenetwork/accumulate/accumulated:develop"
}

variable "bvns" {
  type = number
  default = 3
}

variable "validators" {
  type = number
  default = 4
}

variable "aws_profile" {
  type = string
  default = "default"
}

variable "aws_region" {
  type = string
  default = "us-east-1"
}