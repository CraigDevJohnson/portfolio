output "environment" {
  value = module.service.environment
}

output "image_uri" {
  value = module.service.image_uri
}

output "lambda_function_name" {
  value = module.service.lambda_function_name
}

output "lambda_function_arn" {
  value = module.service.lambda_function_arn
}

output "lambda_published_version" {
  value = module.service.lambda_published_version
}

output "lambda_alias_name" {
  value = module.service.lambda_alias_name
}

output "lambda_alias_arn" {
  value = module.service.lambda_alias_arn
}

output "lambda_execution_role_name" {
  value = module.service.lambda_execution_role_name
}

output "lambda_execution_permissions_boundary_arn" {
  value = module.service.lambda_execution_permissions_boundary_arn
}

output "lambda_runtime_policy_name" {
  value = module.service.lambda_runtime_policy_name
}

output "api_id" {
  value = module.service.api_id
}

output "api_default_url" {
  value = module.service.api_default_url
}

output "api_name" {
  value = module.service.api_name
}

output "lambda_log_group_name" {
  value = module.service.lambda_log_group_name
}

output "api_access_log_group_name" {
  value = module.service.api_access_log_group_name
}

output "google_connection_table_name" {
  value = module.service.google_connection_table_name
}

output "google_connection_table_arn" {
  value = module.service.google_connection_table_arn
}

output "soccer_session_table_name" {
  value = module.service.soccer_session_table_name
}

output "soccer_session_table_arn" {
  value = module.service.soccer_session_table_arn
}

output "ssm_parameter_paths" {
  value = module.service.ssm_parameter_paths
}

output "alarm_arns" {
  value = module.service.alarm_arns
}

output "alarm_names" {
  value = module.service.alarm_names
}

output "certificate_arn" {
  value = module.service.certificate_arn
}

output "acm_validation_records" {
  value = module.service.acm_validation_records
}

output "api_gateway_domain_targets" {
  value = module.service.api_gateway_domain_targets
}

output "oauth_redirect_uris" {
  value = module.service.oauth_redirect_uris
}
