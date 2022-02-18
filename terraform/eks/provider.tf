terraform {
  required_providers {
          docker = {
      source     = "kreuzwerker/docker"
      version    = "2.15.0"
    }
      aws = {
          version = "~> 3.72"
      }
      gitlab = {
      source  = "gitlabhq/gitlab"
      version = "3.6.0"
    }
    tls = {
        version = "~> 2.2"
    }
    cloudinit = {
        version = "~> 2.0"
    }
    kubernetes = {
        source = "hashicorp/kubernetes"
        version = "2.8.0"
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

provider "aws" {
  # shared_credentials_file = "~/.aws/credentials"
  profile                 = "myAWS"
  region                  = var.region
}

provider "kubernetes" {
    host = data.aws_eks_cluster.cluster.endpoint
    cluster_ca_certificate = base64decode(data.aws_eks_cluster.cluster.certificate_authority.0.data)
    token                  = data.aws_eks_cluster_auth.cluster.token
    
}