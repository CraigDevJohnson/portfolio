# Temporary production bootstrap candidate

`portfolio-production-bootstrap-candidate.json` is an additive, temporary customer managed policy for the existing human PortfolioDeployer permission set. It is not installed. It must not replace the existing inline policy, restore removed development setup statements, or attach to a runtime/CI role. Every grant expires before **2026-09-28T06:43:37Z**. Detach it after bootstrap even if it has expired. Expiration removes only this policy's grants; existing standing grants remain unchanged.

The candidate is 5,541 compact JSON bytes, below the 6,144-character managed-policy limit. Current development baseline files are unchanged. The candidate adds production state/lock access, exact production execution-role creation/inline policy constrained by the existing permissions boundary, Lambda-only pass-role, exact production Lambda/table/log/alarm setup, a name-constrained API create and root API read, and regional parameter metadata listing. No application data reads/writes, secret value reads/writes, ECR actions, execution-boundary updates, role trust updates, policy attachments, resource deletion (except the exact lock object), DNS, SNS, or certificate/domain setup is granted.

## Stages and limits

1. **Inventory and API identity capture.** State bucket location/versioning, KMS alias metadata and regional log-group metadata are already granted by the retained inline policy; this additive policy does not duplicate them. Verify state versioning and existing resource ownership before planning. API creation is limited by `apigateway:Request/ApiName` to `portfolio-lambda-prod-http`. API IDs are allocated by AWS; API subresource writes and tag writes are intentionally absent until the exact new ID is captured. The name is not globally unique, so verify absence and state ownership before create. Use a reviewed, narrowly staged saved plan; this candidate is not sufficient for a blind full-service apply.
2. **Refine to captured API ID.** Replace the create grant and name-filtered read with exact `/apis/<captured-id>` and reviewed integration/route/stage/tag-resource ARNs and methods. Authorize only the provider operations shown by the saved plan. The service reference does not support the API-name condition on every nested collection, so do not assume it secures wildcard child writes. Tagged CreateApi may require a separate dependent tag authorization; if so, keep the operation blocked until its exact supported request is reviewed, rather than granting unscoped tag writes.
3. **Direct service bootstrap.** Use the existing domain flags `request_custom_domain=false` and `activate_custom_domain=false`. Exact named resources are authorized, but a live reviewed plan and successful provider read-back remain required. Role creation requires the existing boundary. `PutRolePolicy` can replace any inline policy on that one role, bounded by the boundary; IAM does not constrain the policy document to the authored runtime JSON. The reviewed plan supplies that control. `AddPermission` is limited to the production `live` alias and API Gateway principal; the saved plan must bind the invocation SourceArn to the captured production API because the Lambda IAM action exposes no SourceArn request condition. No role deletion, trust update, boundary removal or arbitrary pass-role is granted.
4. **Runtime material separately.** The actual source and runtime boundary use `/portfolio/lambda/prod/CLIENT_ID_KEY`, `/portfolio/lambda/prod/CLIENT_SECRET_KEY`, `/portfolio/lambda/prod/LPS_SESSION_KEY`. A previous empty query at `/portfolio/prod/` did not establish absence. A September 27 named `aws-setup-audit` metadata query against the correct prefix was denied, so live existence remains unknown. `ssm:DescribeParameters` requires Resource `*`; this candidate permits regional metadata enumeration, not parameter values. Apply an exact Name filter in the read-only query and report only name/type/key/version metadata. If absent, prepare a separate short-lived operation granting only PutParameter on those three exact ARNs and required encryption through `ssm.us-west-2.amazonaws.com`, with parameter encryption context and the actual key. Never grant GetParameters/GetParameterHistory/GetParametersByPath or copy legacy encrypted connection data. Obtain values through the approved secret-handling path without exposing them to logs or agent context.
5. **Domains later.** No ACM or API Gateway domain/mapping privileges are included. Request/validate the exact apex/www certificate in a later phase, then grant reads only to its captured ARN. The repository's domain-bootstrap record documents that DomainNames collection creation cannot be constrained by request hostname or request tags; do not pretend a wildcard domain-create grant is exact. A later approved saved plan, supported Regional/TLS conditions, exact encoded tag-resource grant and immediate removal are required. DNS stays a separate reviewed operation. SNS alert-target creation/subscription and any service-linked role are likewise outside this candidate.

## Validation and sources

Static contract tests cover expiration, exact state/lock, no secret access/destruction, role boundary/pass-role and staged API scope. Existing baseline hash tests protect unchanged development documents. AWS Access Analyzer ValidatePolicy was attempted through `aws-setup-audit` and denied; no live policy validation or effective-access claim is made. A permitted metadata validator must complete it before installation. Provider-dependent grants and actual live allowed/denied tests remain required.

This task starts from retained policies and native `.tf` configuration, not a rendered Terraform plan. Native `.tf` is not an IAM Policy Autopilot input. After a legitimate plan exists, use the AWS IAM skill's plan workflow to compare provider-required actions with this candidate, without uploading generated policies:

```sh
uvx iam-policy-autopilot@latest generate-policies /absolute/private/production-plan.json \
  --region us-west-2 --account 180294223248 --pretty
```

References checked September 27, 2026:

- [API Gateway V2 actions/resources/conditions](https://docs.aws.amazon.com/service-authorization/latest/reference/list_apigatewayv2.html): API name conditions differ from nested resource conditions.
- [IAM actions/resources/conditions](https://docs.aws.amazon.com/service-authorization/latest/reference/list_iam.html): CreateRole/PutRolePolicy boundary conditions and PassRole service restriction.
- [Lambda actions/resources/conditions](https://docs.aws.amazon.com/service-authorization/latest/reference/list_lambda.html): AddPermission supports `lambda:Principal`, not a request SourceArn restriction.
- [Systems Manager actions/resources](https://docs.aws.amazon.com/service-authorization/latest/reference/list_ssm.html): DescribeParameters has no resource-level ARN support.
- [Repository bootstrap history](../README.md): preserve removed temporary statements; domain/tag-condition limitations are documented live evidence.

## Exact API identity stage

The reviewed execution uses a private single-resource OpenTofu configuration
with the final address `module.service.aws_apigatewayv2_api.app`, the existing
production backend/key and the pinned provider. It creates only the named HTTP
API with the final four ownership tags. Before applying, require absent
production state and a saved plan containing exactly this one create, no data
or other managed resources, no updates or deletes and no secret inputs.

The temporary first-stage policy adds only name-constrained API creation and
write access to the exact production state object; the existing temporary
planning policy supplies metadata and lock access. The pinned provider sends
tags in CreateApi and reads them back in GetApi. After capturing the ID, withdraw
creation authority and refine subsequent resource grants to that exact API.
The complete owner configuration then uses the same state/address and must show
that API unchanged. Never run the single-resource stage again after other
production resources have entered state. This avoids targeted or intentionally
failed partial application and does not create a second state owner.
