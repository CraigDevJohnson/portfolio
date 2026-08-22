provider "aws" {
  region = "us-west-2"

  default_tags {
    tags = {
      Project   = "portfolio"
      Platform  = "lambda-http-api"
      ManagedBy = "opentofu"
    }
  }
}
