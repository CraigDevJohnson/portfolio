provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Environment = var.environment
      ManagedBy   = "opentofu"
      Platform    = "lambda-http-api"
      Project     = "portfolio"
    }
  }
}
