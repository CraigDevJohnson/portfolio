provider "aws" {
  region              = "us-west-2"
  allowed_account_ids = [var.aws_account_id]

  default_tags {
    tags = {
      Environment = "dev"
      ManagedBy   = "opentofu"
      Platform    = "site-identity"
      project     = "portfolio"
    }
  }
}
