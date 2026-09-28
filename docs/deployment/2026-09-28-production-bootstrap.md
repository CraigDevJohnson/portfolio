# Production bootstrap, September 28, 2026

The approved foundation continuation provisioned the current-account portfolio
origin in `180294223248`, Oregon. This is bootstrap evidence, not public launch
acceptance. Public apex DNS still points to the existing site at this checkpoint;
real Soccer/Calendar acceptance and protected production promotion remain open.

## Verified resources and source

The selected image remains the development-verified source
`b90d7cf9fc28377341b44546482415c9e42e4276`, digest
`sha256:f1f3305e3deac13d588fb84b9b2e77b5a99284a1b3ddfb35c525c68d8751ae0d`.
[Development recovery run 36356429868](https://github.com/CraigDevJohnson/portfolio/actions/runs/36356429868)
succeeded. Production has an observed bootstrap alias at version 1; it has no
verified production predecessor or automatic rollback target.

The production root owns 24 managed addresses: the 18 service resources, one
public certificate, its validation record, two Regional domains and two root API
mappings. Fresh plans after service creation and after domain activation both
reported no changes. The API is `nryvjx7x90`. The apex mapping is `aee9e1` and the
www mapping is `kxizvr`; both route to that API's `$default` stage.

The origin, home, Soccer, stylesheet and image probes returned 200 with expected
content types. Direct custom-domain probes retained the actual hostname for TLS
verification and returned the exact selected revision at `/healthz`. Both domain
configurations report `AVAILABLE`, `REGIONAL`, `TLS_1_2` and `API_MAPPING_ONLY`.
The certificate is Amazon-issued, DNS-validated for both names, and nonexportable.

Three isolated standard SecureStrings were created with AWS-managed encryption.
The existing approved Google client was reused; the production session key was
newly generated. Exact encrypted readback matched all three values. Values stayed
in process memory and were not written to logs, state, Git or evidence files.
Production tables are fresh; legacy encrypted connections were not copied.

## Operator stages and permissions

Saved plans, their JSON/text, checksums, input provenance and live readbacks are
retained in the private continuation evidence directory outside Git. Service
operations used the normal `portfolio-deployer` SSO identity. The isolated root
login was used for necessary bounded IAM bootstrap and otherwise unavailable
metadata. Every temporary grant remains within the original change window ending
September 28 at 06:43:37 UTC.

- The API was created at its final address in the existing production backend.
  AWS required wildcard tag authorization before assigning its ID, so the shell
  was created without tags, then tagged under exact-ID authority. A denied
  post-create read left a tainted state record; live identity/configuration were
  verified before removing that state marker. No replacement API was created.
- API tagging required `PATCH` on the exact API and `POST` on its unencoded tag
  resource. Name-based read conditions and encoded-only tag grants were not
  sufficient. The final owner configuration and state converged.
- The full service plan created the remaining resources except the stage.
  Tagged stage creation requested an action Access Analyzer reports as invalid;
  it was not granted. The exact stage was created without tags using the reviewed
  access-log settings, tagged, then imported at its final owner address. A short
  regional log-delivery grant was necessary because those APIs do not support
  resource-level IAM scopes. Existing log resource policies did not change.
- Certificate creation and custom-domain activation were separate saved plans.
  Domain collection creation cannot be restricted by request hostname in IAM.
  Its short-lived Regional/TLS conditions, exact source, checked saved plan,
  checksum and prompt removal bound the two intended names. No delete or
  certificate-export authority was granted.
- Service provisioning, secret transfer, log-delivery setup, certificate request
  and domain creation grants were removed after their respective readbacks.
  At this checkpoint, the temporary bootstrap policy permits only exact API,
  certificate, domain and captured mapping metadata. The separate planning
  policy retains temporary metadata/state-read and exact lock access. Both
  temporary attachments and the isolated root login still require final closeout.

AWS IAM Policy Autopilot was run against saved plan JSON as a baseline comparison.
Its broad deletion, unrelated tagging, data-plane and wildcard suggestions were
not installed. Service authorization documentation, actual provider requests,
Access Analyzer and independent IAM readbacks supported the bounded policies.

## Public routing preparation

The two exact ACM validation CNAMEs were added as DNS-only records and resolved
publicly before certificate issuance. Existing records were preserved.
Cloudflare's existing active WWW rule already performs a 301 redirect using the
full URI. Public probes confirmed path and query preservation.

Zone-wide Always Use HTTPS was off and was left unchanged. A new rule matches
only plain HTTP requests to the apex and www production hosts and returns 308 to
`https://craigdevjohnson.com` with the original path and query. The existing dev
rule and other hostnames were preserved. Live probes verified HTTP apex and www
308 responses and the existing HTTPS www 301 response. The apex traffic CNAME
has not yet been switched at this checkpoint.

## Applicable baseline review

| Controls | Evidence and remaining gate |
| --- | --- |
| FND-01 | Current-account first launch is the accepted placement. Portfolio owns application resources; shared alarm delivery remains foundation-owned. Member migration is later. |
| IAM-01 | Named SSO service operations, bounded temporary grants, exact state locks and independent IAM comparisons. Final temporary-access/root-session closeout remains required. |
| IAM-02 | Root MFA is enabled. One inactive root key remains under Craig's explicitly approved first-launch exception through October 5 at 06:33 UTC. Deletion review is not before October 4 at 06:33:23 UTC. This is a gap, not compliance; Foundry measurement still requires zero root keys. |
| ORG-01 | No member safeguards were changed. Management resources are not protected by member SCPs. |
| PUB-01, PUB-02 | The intended public surface is the reviewed HTTP API/custom domains. No public S3 store, VM, SSH/RDP ingress, AMI or snapshot was created. |
| ENC-01 | Encrypted SecureStrings and tables; verified public-certificate HTTPS and Regional TLS 1.2. No new customer-managed encryption key was needed. |
| LOG-01 | Existing organization logging remains foundation-owned; this change did not alter its selectors or archive. |
| LOG-02 | Both new application log groups use 30 days; old log retention was not changed. API access logging is configured. |
| OPS-01 | Five exact alarms use the existing encrypted Ohio foundation route. Full-pattern validation, targets and native selector tests passed. Application alarm-to-inbox proof remains a pre-cutover gate. |
| DET-01 | Live Oregon and Ohio detector reads show enabled foundational coverage, S3 data protection and Lambda network protection. Unselected plans remain disabled. |
| REC-01, REC-02 | PITR and deletion protection are enabled. Complete Ohio restoration, scheduled recovery history and later member cutovers remain owner-local migration gates; PITR does not prove them. |
| FIN-01 | Existing consolidated cost monitoring remains. Incremental rates and an explicit usage scenario appear below; actual usage and native cost-email delivery are separate evidence. |
| EVD-01, EVD-02 | Private saved-plan/live evidence, repository contract checks, live readbacks and free policy validation. No paid analysis service was enabled. |

## Incremental cost model

These are usage inputs, not a spending cap or a claim about measured future
traffic. The illustration assumes 100,000 requests/month, 250 ms average billed
Lambda duration at 512 MB, 1 million DynamoDB read units, 100,000 write units,
1 GB combined table/PITR storage, 1 GB log ingestion and retained log storage,
and 0.1 GB of GuardDuty Lambda network analysis. Shared free tiers are ignored.

| Component | Rate and illustrative monthly amount |
| --- | --- |
| Lambda | $0.20/million requests plus $0.0000166667/GB-second: about $0.23. Reserved concurrency is not provisioned concurrency. |
| HTTP API | $1/million request units at the first tier: $0.10. |
| Five standard alarms | $0.10 each: $0.50. |
| Logs | $0.50/GB ingestion plus $0.03/GB-month storage: $0.53. |
| DynamoDB standard on-demand | $0.125/million read units, $0.625/million write units, $0.25/GB-month storage and $0.20/GB-month PITR: about $0.64. |
| GuardDuty Lambda network analysis | First paid tier $1/GB: $0.10 for the stated input. |
| Standard parameters, public nonexportable certificate, existing Cloudflare free plan | No new fixed subscription/resource charge. |
| Existing immutable image and shared notification route | Reused; no duplicate repository image, SNS topic or encryption key was created. Request, delivery and transfer usage remains metered. |

The stated compute, API, storage, logs, alarms and analysis scenario totals about
**$2.10/month before free tiers**, excluding usage-dependent internet/cross-region
transfer, KMS requests, shared notification/query requests and existing
foundation/legacy overlap. Those exclusions must be included in measured account
costs; this subtotal is not an all-in forecast. No legacy service has been stopped
or deleted by this bootstrap. Recovery trials and retirement have separate
cost/observation gates.

Rates were checked September 28 against primary sources:
[Lambda pricing](https://aws.amazon.com/lambda/pricing/),
[HTTP API pricing](https://aws.amazon.com/api-gateway/pricing/),
[Oregon CloudWatch offer](https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonCloudWatch/current/us-west-2/index.json),
[Oregon DynamoDB offer](https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonDynamoDB/current/us-west-2/index.json),
[Oregon GuardDuty offer](https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonGuardDuty/current/us-west-2/index.json),
[Parameter Store pricing](https://aws.amazon.com/systems-manager/pricing/), and
[ACM pricing](https://aws.amazon.com/certificate-manager/pricing/).
