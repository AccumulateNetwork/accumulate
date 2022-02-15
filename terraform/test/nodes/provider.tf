terraform {
  required_providers {
          docker = {
      source     = "kreuzwerker/docker"
      version    = "2.15.0"
    }
      aws = {
          version = "~> 2.0"
      }
      gitlab = {
      source  = "gitlabhq/gitlab"
      version = "3.6.0"
    }
  }
}

provider "docker" {
  host = "unix:///var/run/docker.sock"
}

provider "gitlab" {
    token = var.gitlab_token
    base_url = "https://gitlab.com/accumulatenetwork/accumulate"
}

variable "gitlab_token" {
  type = string
}

provider "aws" {
  # shared_credentials_file = "~/.aws/credentials"
  profile                 = "myAWS"
  region                  = "us-east-1"
}


  terraform {
  backend "http" {
  }
}