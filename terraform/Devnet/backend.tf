resource "aws_s3_bucket" "devnet_state" {
  bucket = "accumulate-devnet-state"

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

resource "aws_dynamodb_table" "devnet_locks" {
  name         = "accumulate-devnet-state-locking"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }
}

# backend configuration comment out before you run 'terraform init' for the first time

# terraform {
#  backend "s3" {
#     bucket         = "accumulate-devnet-state"
#     key            = "devnet/s3/terraform.tfstate"
#     region         = "us-east-1"
#     profile        = "myAWS"
#     dynamodb_table = "accumulate-devnet-state-locking"
#     encrypt        = true
#   }
# }


resource "aws_cloudwatch_log_group" "devnet_log" {
  name = "accumulate-devnet-log"
}