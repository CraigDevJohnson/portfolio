terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "= 5.100.0"
    }
  }

  # Retired root. Its live resources are destroyed from the management-final
  # tag (refactor Phase 14); never apply this root in the workloads account.
  # retired.tf fails every plan here.
  backend "s3" {
    bucket       = "portfolio-tofu-state-793680745829"
    key          = "portfolio/terraform.tfstate"
    region       = "us-west-2"
    use_lockfile = true
    encrypt      = true
  }
}

provider "aws" {
  region              = var.aws_region
  allowed_account_ids = [var.aws_account_id]

  default_tags {
    tags = {
      project   = "portfolio"
      ManagedBy = "opentofu"
    }
  }
}
