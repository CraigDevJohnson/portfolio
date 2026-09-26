output "cognito_user_pool_id" {
  value = module.site.cognito_user_pool_id
}

output "google_redirect_uri" {
  value = module.site.google_redirect_uri
}

output "session_parameter_path" {
  value = local.session_parameter_path
}

output "site_runtime" {
  description = "Reviewed non-secret production settings for SITE_* Lambda configuration."
  value = {
    cognito_domain       = module.site.cognito_domain
    cognito_issuer       = module.site.cognito_issuer
    cognito_client_id    = module.site.cognito_client_id
    redirect_uri         = module.site.callback_uri
    logout_uri           = module.site.logout_uri
    invitations          = local.invitations
    allow_local_callback = module.site.allow_local_callback
  }
}
