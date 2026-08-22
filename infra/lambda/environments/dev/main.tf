module "service" {
  source = "../../modules/service"

  environment                = var.environment
  name_prefix                = var.name_prefix
  aws_region                 = var.aws_region
  ecr_repository_url         = var.ecr_repository_url
  image_digest               = var.image_digest
  lambda_memory_mb           = var.lambda_memory_mb
  lambda_timeout_seconds     = var.lambda_timeout_seconds
  reserved_concurrency       = var.reserved_concurrency
  log_retention_days         = var.log_retention_days
  enable_pitr                = var.enable_pitr
  enable_deletion_protection = var.enable_deletion_protection
  alarm_action_arns          = var.alarm_action_arns
  domain_names               = var.domain_names
  request_custom_domain      = var.request_custom_domain
  activate_custom_domain     = var.activate_custom_domain
  live_version_override      = var.live_version_override
}
