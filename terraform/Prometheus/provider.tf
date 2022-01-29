terraform {
  required_providers {
          docker = {
      source     = "kreuzwerker/docker"
      version    = "2.15.0"
    }
      aws = {
          version = "~> 2.0"
      }
  }
}

provider "docker" {
  host = "unix:///var/run/docker.sock"
}


provider "aws" {
  profile                 = "myAWS"
  region                  = "us-east-1"
}