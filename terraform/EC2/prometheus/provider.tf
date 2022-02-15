terraform {
  required_providers {
      aws = {
          version = "~> 2.0"
      }
  }
}

provider "aws" {
  # shared_credentials_file = "~/.aws/credentials"
  profile                 = "myAWS"
  region                  = "us-east-1"
}



