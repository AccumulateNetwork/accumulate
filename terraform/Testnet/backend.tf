# backend configuration comment out before you run 'terraform init' for the first time

# terraform {
#  backend "s3" {
#     bucket         = "accumulate-testnet-state-testnet"
#     key            = "testnet/s3/terraform.tfstate"
#     region         = "us-east-1"
#     profile        = "myAWS"
#     dynamodb_table = "accumulate-testnet-state-locking"
#     encrypt        = true
#   }
# }

resource "aws_s3_bucket" "testnet_state" {
  bucket = "accumulate-testnet-state-testnet"

  lifecycle {
      prevent_destroy = false
  }

  versioning {
    enabled = true
  }

  server_side_encryption_configuration {
    rule {
        apply_server_side_encryption_by_default {
          sse_algorithm = "AES256"
        }
    }
  }
}

resource "aws_dynamodb_table" "testnet_locks" {
  name         = "accumulate-testnet-state-locking"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }
}

resource "aws_cloudwatch_log_group" "testnet_log" {
  name = "accumulate-testnet-log"
}