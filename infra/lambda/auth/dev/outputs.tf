output "cognito_user_pool_id" {
  description = "Development management Cognito user-pool ID."
  value       = aws_cognito_user_pool.management.id
}

output "cognito_domain" {
  description = "Development management Cognito managed-login origin."
  value       = local.cognito_domain
}

output "cognito_issuer" {
  description = "Development management Cognito user-pool issuer."
  value       = local.cognito_issuer
}

output "cognito_client_id" {
  description = "Development management public app-client ID."
  value       = aws_cognito_user_pool_client.management.id
}

output "google_redirect_uri" {
  description = "Google OAuth redirect URI owned by Cognito."
  value       = local.google_redirect_uri
}

output "session_parameter_path" {
  description = "SecureString parameter name whose value is injected outside this root."
  value       = local.session_parameter_path
}

output "management_runtime" {
  description = "Reviewed public settings for the development runtime handoff."
  value = {
    cognito_domain           = local.cognito_domain
    cognito_issuer           = local.cognito_issuer
    cognito_client_id        = aws_cognito_user_pool_client.management.id
    redirect_uri             = local.callback_uri
    logout_uri               = local.logout_uri
    allowed_emails           = local.allowed_emails
    allow_local_callback     = var.enable_local_callback
    ec2_management_tag_key   = local.ec2_management_tag_key
    ec2_management_tag_value = local.ec2_management_tag_value
  }
}
