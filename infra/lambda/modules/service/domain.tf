locals {
  sorted_domain_names = sort(tolist(var.domain_names))
  acm_validation_records_by_domain = var.request_custom_domain ? {
    for option in aws_acm_certificate.custom[0].domain_validation_options : option.domain_name => {
      domain_name           = option.domain_name
      resource_record_name  = option.resource_record_name
      resource_record_type  = option.resource_record_type
      resource_record_value = option.resource_record_value
    }
  } : {}
  acm_validation_records = slice(tolist(concat(
    [
      for domain_name in sort(keys(local.acm_validation_records_by_domain)) :
      local.acm_validation_records_by_domain[domain_name]
    ],
    [{
      domain_name           = ""
      resource_record_name  = ""
      resource_record_type  = ""
      resource_record_value = ""
    }],
  )), 0, length(local.acm_validation_records_by_domain))
  empty_string_list = slice(tolist([""]), 0, 0)
  api_gateway_domain_targets = var.activate_custom_domain ? tomap({
    for domain_name in var.domain_names : domain_name =>
    aws_apigatewayv2_domain_name.custom[domain_name].domain_name_configuration[0].target_domain_name
  }) : zipmap(local.empty_string_list, local.empty_string_list)
}

resource "aws_acm_certificate" "custom" {
  count = var.request_custom_domain ? 1 : 0

  domain_name               = local.sorted_domain_names[0]
  subject_alternative_names = slice(local.sorted_domain_names, 1, length(local.sorted_domain_names))
  validation_method         = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_acm_certificate_validation" "custom" {
  count = var.activate_custom_domain ? 1 : 0

  certificate_arn         = aws_acm_certificate.custom[0].arn
  validation_record_fqdns = [for record in values(local.acm_validation_records_by_domain) : record.resource_record_name]
}

resource "aws_apigatewayv2_domain_name" "custom" {
  for_each = var.activate_custom_domain ? var.domain_names : toset([])

  domain_name = each.value

  domain_name_configuration {
    certificate_arn = aws_acm_certificate_validation.custom[0].certificate_arn
    endpoint_type   = "REGIONAL"
    security_policy = "TLS_1_2"
  }
}

resource "aws_apigatewayv2_api_mapping" "custom" {
  for_each = var.activate_custom_domain ? var.domain_names : toset([])

  api_id      = aws_apigatewayv2_api.app.id
  domain_name = aws_apigatewayv2_domain_name.custom[each.key].id
  stage       = aws_apigatewayv2_stage.default.id
}
