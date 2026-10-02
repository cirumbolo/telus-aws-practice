terraform {
  required_version = ">= 1.10" # import ids with variables (1.6+), S3 native locking (1.10+)

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region
}
