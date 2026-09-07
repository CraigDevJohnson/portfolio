provider "aws" {
  region              = "us-west-2"
  allowed_account_ids = ["180294223248"]

  default_tags {
    tags = {
      Environment = "dev"
      ManagedBy   = "opentofu"
      Platform    = "cognito-auth"
      Project     = "portfolio"
    }
  }
}
