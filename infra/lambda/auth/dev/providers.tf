provider "aws" {
  region              = local.aws_region
  allowed_account_ids = [local.aws_account_id]

  default_tags {
    tags = {
      Environment = "dev"
      ManagedBy   = "opentofu"
      Platform    = "cognito-auth"
      Project     = "portfolio"
    }
  }
}
