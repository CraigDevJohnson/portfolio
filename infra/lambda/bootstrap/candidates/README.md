# Installed development management policy inputs

These exact files were installed and independently verified on September 7,
2026. Their candidate filenames and hashes are retained unchanged; they are now
the current installed inputs. The original JSON documents in the parent
directory remain pre-Cognito baselines, not installation defaults. Presence in
Git does not authorize another installation or live use, and CI permissions
remain unchanged. The auth setup policy is attached only to the human deployer;
never attach it to a runtime or CI role. None of these documents grants
permission to install or edit the execution boundary.

The setup managed policy is version `v1` at
`/portfolio/setup/PortfolioAuthDevelopmentSetup`; the management document is the
`PortfolioDeployer` inline policy; the boundary management document is default
version `v2`. SSO configuration and effective deployer role policies both match
the reviewed documents. The boundary is attached only to the development
execution role. Temporary administrator installation authority was removed and
the administrator's SSO and effective role inline policies restored to the
reviewed read-only document, with no managed-policy attachments.

The boundary candidate preserves every Prod statement and every existing Dev grant; its additions permit only the session parameter and reviewed management operations. The development bootstrap candidate preserves its complete baseline and adds session injection permissions. The separate setup candidate grants encrypted auth state access and tag-constrained development Cognito create/update/read setup. It excludes Cognito delete actions. Dependent TagResource permits only the four exact creation tags and refuses to overwrite conflicting existing scope tags; GetUserPoolMfaConfig supports the pinned provider read path. CreateUserPool and DescribeUserPoolDomain require wildcard resources; they are restricted by region and, for creation, exact request tags. After creation, pin the pool ARN before an installation review if a narrower ongoing scope is wanted.

| File | SHA-256 | Bytes | Non-whitespace bytes |
| --- | --- | ---: | ---: |
| portfolio-deployer-auth-development-setup-candidate.json | `69eca6d9d1bca4b6d444200fc18e56e20f5c1256f7454c2d4de9e04ccbcf2811` | 4116 | 2951 |
| portfolio-deployer-development-management-candidate.json | `68b54a348a07e7a27cda04499c08c79936e0e54a3cbb2887c3bb9c068d9af613` | 8975 | 6606 |
| portfolio-lambda-execution-boundary-management-candidate.json | `31ceb135cf378a4b033f51bba6192d01d74b140f58a409a48621f784499be8dc` | 6957 | 5208 |

AWS Identity Center inline-policy size limits apply to the combined permission-set document. Keep the separate auth setup policy separately managed and review the combined effective grants and permissions boundary before any later
installation. No candidate grants EC2 tagging, reboot, production portal access, auth-backend access to CI, or Google/session credentials in runtime state.

The development workflow reads the public GitHub environment variable `MANAGEMENT_RUNTIME_JSON` as `EXPECTED_MANAGEMENT_JSON`, defaulting to JSON `null`. It must contain only the reviewed bare management object (extract `.management` from the Task 5 export), never Google credentials or a session key. The ordinary release gate rejects configuration changes even when this input is configured.

On September 7, 2026, all three exact documents passed live Access Analyzer
`IDENTITY_POLICY` validation with zero findings through the non-root
`portfolio-auth-policy-admin` profile. Installation read-back confirmed the
candidate contents, unchanged production statements and expected attachments.
The ordinary `portfolio-deployer` profile subsequently passed bucket-security,
auth-prefix and domain metadata reads. These checks supersede the earlier
denials but do not establish a successful Cognito plan or apply. The
[external setup record](../../../../docs/deployment/2026-09-07-cognito-external-setup-review.md)
records installation and remaining provisioning approvals.

Tagged user-pool creation and provider reads were cross-checked against the
[AWS Cognito IAM operations reference](https://docs.aws.amazon.com/service-authorization/latest/reference/list_cognito-idp.html)
and [pinned AWS provider v6.38.0](https://github.com/hashicorp/terraform-provider-aws/blob/v6.38.0/internal/service/cognitoidp/user_pool.go).
