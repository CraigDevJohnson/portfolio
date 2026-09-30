# The account root is administered only from Craig's WorkloadsAdmin session.
# Release workflows never plan or apply it.
provider "aws" {
  region              = local.region
  profile             = "workloads-admin"
  allowed_account_ids = [var.aws_account_id]

  default_tags {
    tags = {
      ManagedBy = "opentofu"
      project   = "portfolio"
    }
  }
}
