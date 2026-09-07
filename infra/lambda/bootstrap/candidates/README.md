# Unapproved development management policy candidates

These files are review artifacts only. They do not replace the approved bootstrap artifacts, authorize installation, or change CI permissions. The auth setup candidate is a separately attached human setup-role policy; never attach it to a runtime or CI role. No candidate grants permission to install or edit the execution boundary.

The boundary candidate preserves every Prod statement and every existing Dev grant; its additions permit only the session parameter and reviewed management operations. The development bootstrap candidate preserves its complete baseline and adds session injection permissions. The separate setup candidate grants encrypted auth state access and tag-constrained development Cognito create/update/read setup. It excludes Cognito delete actions. Dependent TagResource permits only the four exact creation tags and refuses to overwrite conflicting existing scope tags; GetUserPoolMfaConfig supports the pinned provider read path. CreateUserPool and DescribeUserPoolDomain require wildcard resources; they are restricted by region and, for creation, exact request tags. After creation, pin the pool ARN before an installation review if a narrower ongoing scope is wanted.

| File | SHA-256 | Bytes | Non-whitespace bytes |
| --- | --- | ---: | ---: |
| portfolio-deployer-auth-development-setup-candidate.json | `69eca6d9d1bca4b6d444200fc18e56e20f5c1256f7454c2d4de9e04ccbcf2811` | 4116 | 2951 |
| portfolio-deployer-development-management-candidate.json | `68b54a348a07e7a27cda04499c08c79936e0e54a3cbb2887c3bb9c068d9af613` | 8975 | 6606 |
| portfolio-lambda-execution-boundary-management-candidate.json | `31ceb135cf378a4b033f51bba6192d01d74b140f58a409a48621f784499be8dc` | 6957 | 5208 |

AWS Identity Center inline-policy size limits apply to the combined permission-set document. Keep the separate auth setup policy separately managed and review the final combined attachment and permissions boundary before installation. No candidate grants EC2 tagging, reboot, production portal access, auth-backend access to CI, or Google/session credentials in runtime state.

The development workflow reads the public GitHub environment variable `MANAGEMENT_RUNTIME_JSON` as `EXPECTED_MANAGEMENT_JSON`, defaulting to JSON `null`. It must contain only the reviewed bare management object (extract `.management` from the Task 5 export), never Google credentials or a session key. The ordinary release gate rejects configuration changes even when this input is configured.

Access Analyzer validation was attempted read-only and denied by current permissions for all three candidates; policy semantics remain unverified live. Tagged user-pool creation and provider reads were cross-checked against the [AWS Cognito IAM operations reference](https://docs.aws.amazon.com/service-authorization/latest/reference/list_cognito-idp.html) and [pinned AWS provider v6.38.0](https://github.com/hashicorp/terraform-provider-aws/blob/v6.38.0/internal/service/cognitoidp/user_pool.go).
