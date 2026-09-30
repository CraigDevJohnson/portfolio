provider "aws" {
  region              = "us-west-2"
  allowed_account_ids = [var.aws_account_id]

  default_tags {
    tags = {
      project   = "portfolio"
      Platform  = "lambda-http-api"
      ManagedBy = "opentofu"
    }
  }
}
