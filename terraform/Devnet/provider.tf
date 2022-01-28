terraform {
  required_providers {
    docker  = {
      source      = "kreuzwerker/docker"
      version     = "2.15.0"
    }
    aws = {
      version = "3.74.0"
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
    token = backend.config.password
    base_url = "https://gitlab.com/accumulatenetwork/accumulate"
}

provider "aws" {
  profile = var.aws_profile
  region  = var.aws_region
}