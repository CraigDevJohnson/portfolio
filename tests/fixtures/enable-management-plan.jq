def parameter: "arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/dev/MGMT_SESSION_KEY";
def conditions($action):
  if $action == ["kms:Decrypt"] then
    {StringEquals: {"kms:EncryptionContext:PARAMETER_ARN":
      (["CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "LPS_SESSION_KEY", "MGMT_SESSION_KEY"] |
        map("arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/dev/" + .))}}
  else {} end;
def extra: [
  {Effect: "Allow", Action: ["ec2:DescribeInstances", "cloudwatch:GetMetricStatistics"], Resource: "*",
    Condition: {StringEquals: {"aws:RequestedRegion": "us-west-2"}}},
  {Effect: "Allow", Action: "logs:FilterLogEvents",
    Resource: "arn:aws:logs:us-west-2:180294223248:log-group:/ec2/i-*:*"},
  {Effect: "Allow", Action: ["ec2:StartInstances", "ec2:StopInstances"],
    Resource: "arn:aws:ec2:us-west-2:180294223248:instance/*",
    Condition: {StringEquals: {"ec2:ResourceTag/PortfolioManagement": "dev"}}}
];
def array: if type == "array" then . else [.] end;
def data_conditions:
  [to_entries[] | .key as $test | .value | to_entries[] |
    {test: $test, variable: .key, values: (.value | array)}];
def enabled_policy:
  .Statement |= (map(
    if (.Action | array) == ["ssm:GetParameters"] then .Resource += [parameter]
    elif (.Action | array) == ["kms:Decrypt"] then .Condition = conditions(["kms:Decrypt"])
    else . end) + extra);
def enabled_env: . + {
  MGMT_SESSION_KEY: "/portfolio/lambda/dev/MGMT_SESSION_KEY",
  MGMT_COGNITO_DOMAIN: $management.cognito_domain,
  MGMT_COGNITO_ISSUER: $management.cognito_issuer,
  MGMT_COGNITO_CLIENT_ID: $management.cognito_client_id,
  MGMT_COGNITO_REDIRECT_URI: $management.redirect_uri,
  MGMT_COGNITO_LOGOUT_URI: $management.logout_uri,
  MGMT_ALLOWED_EMAILS: ($management.allowed_emails | join(",")),
  MGMT_ALLOW_LOCAL_CALLBACK: ($management.allow_local_callback | tostring),
  MGMT_AWS_REGION: "us-west-2"
};
.variables.management = {value: $management} |
.resource_changes |= map(
  if .address == "module.service.aws_lambda_function.app" then
    .change.after.environment[0].variables |= enabled_env |
    if .change.before != null then .change.before.environment[0].variables |= enabled_env else . end
  elif .address == "module.service.aws_iam_role_policy.lambda" then
    if .change.after.policy != null then .change.after.policy |= (fromjson | enabled_policy | tojson) else . end |
    if .change.before.policy != null then .change.before.policy |= (fromjson | enabled_policy | tojson) else . end
  elif .address == "module.service.data.aws_iam_policy_document.lambda" then
    .change.after.statement |= (map(
      if .actions == ["ssm:GetParameters"] then .resources += [parameter]
      elif .actions == ["kms:Decrypt"] then .condition = (conditions(.actions) | data_conditions)
      else . end) + [extra[] | {
        actions: (.Action | array), resources: (.Resource | array),
        condition: ((.Condition // {}) | data_conditions), effect: null, not_actions: null,
        not_principals: [], not_resources: null, principals: [], sid: null
      }]) |
    del(.change.after_unknown.statement)
  else . end)
