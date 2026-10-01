output "cognito_user_pool_id" {
  value = aws_cognito_user_pool.site.id
}

output "cognito_issuer" {
  value = "https://${aws_cognito_user_pool.site.endpoint}"
}

output "cognito_client_id" {
  value = aws_cognito_user_pool_client.site.id
}

output "cognito_domain" {
  value = local.cognito_domain
}

output "callback_uri" {
  value = local.callback_uri
}

output "logout_uri" {
  value = local.logout_uri
}

output "google_redirect_uri" {
  value = local.google_redirect_uri
}

output "allow_local_callback" {
  value = var.enable_local_callback
}
