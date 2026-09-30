environment            = "prod"
name_prefix            = "portfolio-lambda-prod"
aws_region             = "us-west-2"
lambda_memory_mb       = 512
lambda_timeout_seconds = 29
# Temporarily unreserved (-1): the workloads account's Lambda concurrency
# limit is still 10 while the quota increase is pending, and Lambda keeps 100
# unreserved, so a reservation of 10 can't be applied. The account limit caps
# prod at 10 meanwhile. Set this back to 10 once the limit is at least 110
# (aws-setup tracker #30).
reserved_concurrency       = -1
log_retention_days         = 30
enable_pitr                = true
enable_deletion_protection = true
domain_names               = ["craigdevjohnson.com", "www.craigdevjohnson.com"]
request_custom_domain      = true
activate_custom_domain     = true
