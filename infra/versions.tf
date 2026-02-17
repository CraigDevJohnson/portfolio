terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  # Uncomment and configure after initial apply to migrate state to S3.
  # backend "s3" {
  #   bucket         = "your-tofu-state-bucket"
  #   key            = "portfolio/terraform.tfstate"
  #   region         = "us-east-1"
  #   dynamodb_table = "tofu-state-lock"
  #   encrypt        = true
  # }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project   = "portfolio"
      ManagedBy = "opentofu"
    }
  }
}
