<!-- markdownlint-disable MD013 MD033 -->
# Portfolio ECR consumer and publisher audit

Date: 2026-08-30

Repository base: <code>origin/main</code> at <code>1b245fa4</code>

Audit-time working tree: branch <code>codex/remove-app-runner-retirement-tooling</code> with the App Runner cleanup not yet committed

AWS observation window: 2026-08-30 11:26-11:39 UTC

AWS workload scope: expected account, Region <code>us-west-2</code>; an approved administrative session was used only for read-only calls

## Decision

The audit found no inspected live AWS managed runtime in the authorized Region that currently identifies the legacy private ECR repository <code>portfolio</code> as its image source.

That is narrower than a universal deletion-safety claim:

- The former App Runner service was the only confirmed managed runtime consumer. CloudTrail records its deletion at 2026-08-30 11:14:39 UTC, and a fresh App Runner inventory is empty.
- Every live Lambda image reference found uses the separate replacement repository <code>portfolio-lambda-releases</code>. That repository is active and is not a cleanup candidate.
- The legacy repository remains in the current OpenTofu state, contains six image records, and has a mutable <code>latest</code> tag. Deleting it out of band would create state drift and destroy retained artifacts.
- No active checked-in workflow was found that publishes to <code>portfolio</code>, but one broad infrastructure-execution role is allowed the two repository actions tested, administrative access remains a possible path, and external or previously issued credentials cannot be disproved by this inventory.
- The Region-scoped CloudTrail inventory did not locate a trail or event data store. Its available event history is bounded, so it cannot prove lifetime non-use.

Current evidence is sufficient to classify <code>portfolio</code> as orphaned from the identified live managed services in <code>us-west-2</code>. It is not sufficient to authorize deletion or to claim that no possible caller can pull or publish.

## Authorization and method

All AWS calls in this audit were read-only. The identity preflight verified the expected account and exact Region before any inventory call.

No image was pulled, pushed, tagged, copied, exported, deleted, or scanned by this audit. No AWS resource, policy, state object, role, trail, tag, or configuration was changed. The audit wrote only this Markdown file and did not modify the unrelated cleanup changes already present in the shared worktree.

The inventory covered the repository, remote OpenTofu state, ECR metadata and policies, App Runner, Lambda and published Lambda versions, ECS, EKS, Batch, CodeBuild, CodePipeline, SageMaker, Lightsail containers, Elastic Beanstalk, EC2 and its instance profile, Auto Scaling, launch templates, CloudFormation, IAM principals, and available CloudTrail history.

## Repository evidence

### Legacy repository ownership and publisher remnants

- [infra/main.tf](../../infra/main.tf#L27-L60) declares <code>aws_ecr_repository.app</code> from the default app name <code>portfolio</code>, with mutable tags, scan-on-push, and <code>force_delete = false</code>. It also declares the untagged-image lifecycle policy.
- [infra/outputs.tf](../../infra/outputs.tf#L1-L4) exposes that repository URL specifically for tagging and pushing an image.
- [Taskfile.yaml](../../Taskfile.yaml#L337-L381) contains three legacy internal helpers:
  - <code>_check-aws</code>;
  - <code>_ecr-ensure-repo</code>, which can target-apply the legacy repository and lifecycle policy; and
  - <code>_ecr-docker-push</code>, which builds <code>Dockerfile</code> and pushes <code>portfolio:latest</code>.
- A repository-wide caller search found no task that invokes either ECR helper. <code>task --summary _ecr-docker-push</code> exited 202 with <code>Task "_ecr-docker-push" is internal</code>, so the helper is not a current user-facing task. The commands still document a manual publisher path and should be removed with any later repository retirement.
- The only checked-in workflow, [.github/workflows/ci.yml](../../.github/workflows/ci.yml), grants <code>contents: read</code>, runs local build/test/infrastructure validation, and contains no AWS credential, ECR login, Docker push, App Runner deployment, or Lambda deployment step.
- [docker-compose.yml](../../docker-compose.yml#L1-L14) builds the local image <code>portfolio-app:latest</code>; that name is not an ECR URI and is not evidence of a registry consumer.
- [DEPLOY-INSTRUCTIONS.md](../../DEPLOY-INSTRUCTIONS.md#L295-L298) explicitly preserves the legacy repository and requires this separate consumer audit before a later cleanup decision.

Git history contains earlier deployment work, but history is not a current caller. The current tree still contains enough legacy HCL, output, and internal helper material to recreate or republish the repository if an operator bypasses the documented task surface.

### Replacement Lambda repository is distinct and active

- [infra/lambda/artifacts/main.tf](../../infra/lambda/artifacts/main.tf#L1-L55) separately owns <code>portfolio-lambda-releases</code>, enforces immutable tags, and grants a conditioned Lambda service pull policy only for <code>portfolio-lambda-*</code> functions in this account and Region.
- [Taskfile.yaml](../../Taskfile.yaml#L537-L631) derives and pushes full-SHA release tags only to <code>portfolio-lambda-releases</code>.
- The live <code>portfolio-lambda-dev</code> function, its <code>$LATEST</code> configuration, published version <code>1</code>, and <code>live</code> alias all resolve to:

    ACCOUNT_ID.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@sha256:970bdb10…60c49

No Lambda configuration returned the legacy <code>/portfolio</code> image URI.

## Live ECR evidence

### Repository comparison

| Property | Legacy <code>portfolio</code> | Replacement <code>portfolio-lambda-releases</code> |
| --- | --- | --- |
| Created | 2026-02-18 01:36:36 MST | 2026-08-25 03:56:51 MDT |
| Tag policy | <code>MUTABLE</code> | <code>IMMUTABLE</code> |
| Scan on push | true | true |
| Encryption | AES-256 | AES-256 |
| Repository policy | absent | conditioned Lambda pull policy |
| Live image use found | none | <code>portfolio-lambda-dev</code> |

The similar prefix does not make the repositories interchangeable. Their ARNs, URLs, tag contracts, policies, image digests, owners, publishers, and live consumers are different.

### Legacy images and pull time

<code>describe-images</code> returned six legacy image records: five untagged records and one tagged image:

    tag: latest
    digest: sha256:476e1d61…b75e0f3
    pushed: 2026-04-28T04:08:43.401000-06:00
    last recorded pull: 2026-04-28T04:53:28.037000-06:00

The latest last-recorded pull across all six legacy records was 2026-04-28T04:53:28.209000-06:00. <code>list-pull-time-update-exclusions</code> returned no exclusions.

AWS defines <code>lastRecordedPullTime</code> as the time ECR recorded the last pull, but notes that the value is refreshed at least once per 24 hours and can be less precise under frequent pulls. It is useful inactivity evidence, not an exact request log. See the [Amazon ECR ImageDetail API reference](https://docs.aws.amazon.com/AmazonECR/latest/APIReference/API_ImageDetail.html).

The replacement repository contains four immutable full-Git-SHA tags. Its deployed digest records a pull at 2026-08-30T01:24:54.167000-06:00, which further distinguishes the active Lambda artifact path from the inactive legacy path.

### Repository and registry policies

The legacy repository has:

- no repository policy;
- no registry policy;
- no registry replication rule;
- no pull-through cache rule;
- no repository creation template; and
- the expected lifecycle policy that retains only the last five untagged images.

Absence of a repository policy does not mean absence of access. AWS evaluates ECR repository policies and IAM identity policies, and a same-account identity can be allowed by its IAM policy. See [Private repository policies in Amazon ECR](https://docs.aws.amazon.com/AmazonECR/latest/userguide/repository-policies.html).

## Managed consumer inventory

| Surface | Read-only evidence | Legacy consumer finding |
| --- | --- | --- |
| App Runner | <code>list-services</code> returned an empty list | None now; former confirmed consumer was deleted |
| Lambda | Seven functions total; one image function; <code>$LATEST</code> and version <code>1</code> inspected | None; image function uses <code>portfolio-lambda-releases</code> |
| ECS | No clusters and zero active task definitions | None |
| EKS | No clusters | None |
| Batch | No active job definitions | None |
| CodeBuild | No projects | None |
| CodePipeline | No pipelines | None |
| SageMaker | No models and no endpoints | None |
| Lightsail containers | No container services | None |
| Elastic Beanstalk | No applications and no environments | None |
| Auto Scaling | No Auto Scaling groups | None |
| CloudFormation | Seven active stacks inspected; only the CDK asset ECR repository was present; no exact legacy URI or ARN in current templates | None found |
| EC2 | One unrelated running instance | No configured legacy ECR access or startup reference found |

### Former App Runner consumer

The versioned legacy state immediately before retirement identified:

    service: portfolio
    status: RUNNING
    image: portfolio:latest in the expected account and Region
    image repository type: ECR
    auto deployments: false
    access role: dedicated App Runner ECR access role

CloudTrail event history records a successful <code>DeleteService</code> at 2026-08-30T05:14:39-06:00. Fresh read-back found:

- <code>apprunner list-services</code>: empty;
- the dedicated ECR access role: absent;
- the dedicated instance role: absent; and
- the dedicated runtime-secrets policy: absent.

AWS documents that an App Runner service based on a private same-account ECR image requires an ECR access role. See [How App Runner works with IAM](https://docs.aws.amazon.com/apprunner/latest/dg/security_iam_service-with-iam.html). The service and its dedicated access role are both absent, closing the confirmed managed-consumer path.

### OpenTofu state remains authoritative for the repository

A fresh read of the current remote OpenTofu state contains:

- <code>aws_ecr_repository.app</code> for <code>portfolio</code>;
- <code>aws_ecr_lifecycle_policy.app</code>;
- output <code>ecr_repository_url</code>; and
- no App Runner resource.

The prior version shows the retired App Runner relationship. The latest version proves that App Runner retirement did not retire ECR ownership. Any future repository removal must reconcile this state through a reviewed plan; an out-of-band registry deletion would leave the state and configuration inconsistent.

### EC2 and host-level path

The only EC2 instance observed is an unrelated running host. Its AWS configuration has:

- an attached instance profile;
- no instance user data;
- no Auto Scaling group;
- no launch-template user data in any inspected template version; and
- no exact legacy or replacement ECR URI in those user-data fields.

IAM simulation returned <code>implicitDeny</code> for the attached instance role on all tested legacy pull and push actions: <code>BatchGetImage</code>, <code>GetDownloadUrlForLayer</code>, <code>PutImage</code>, <code>InitiateLayerUpload</code>, <code>UploadLayerPart</code>, and <code>CompleteLayerUpload</code>.

This does not inspect the instance filesystem, shell history, Docker cache, manually installed credentials, or running processes. No host connection was authorized or attempted.

## Publisher and permission audit

Principal-policy simulation tested only <code>ecr:BatchGetImage</code> and <code>ecr:PutImage</code> against the exact legacy repository ARN.

Results that were not implicit deny:

| Principal category | <code>BatchGetImage</code> | <code>PutImage</code> | Interpretation |
| --- | --- | --- | --- |
| Broad CloudFormation execution role | allowed | allowed | Partial latent read/write signal; current CloudFormation templates contain no legacy reference |
| CDK lookup role | allowed | implicit deny | Partial latent read signal; current CloudFormation templates contain no legacy reference |
| AWS Support service-linked role | allowed | implicit deny | Partial service-linked read signal, not evidence of an application consumer |

These are partial permission signals, not complete pull or push simulations. They do not test <code>ecr:GetAuthorizationToken</code>, <code>ecr:GetDownloadUrlForLayer</code>, or the layer-upload actions needed by complete registry workflows. See [Private repository policies in Amazon ECR](https://docs.aws.amazon.com/AmazonECR/latest/userguide/repository-policies.html) and [Logging Amazon ECR actions with AWS CloudTrail](https://docs.aws.amazon.com/AmazonECR/latest/userguide/logging-using-cloudtrail.html).

No IAM user was allowed either tested action. The deleted App Runner access role is no longer present. The current CloudFormation stacks and templates contain no exact legacy repository URI or ARN, so the broad infrastructure-execution role is a permission path rather than evidence of an active publisher.

The IAM simulator does not issue a real service request, and AWS warns that simulation can differ from the live environment under some conditions. See [IAM policy testing with the IAM policy simulator](https://docs.aws.amazon.com/IAM/latest/UserGuide/access_policies_testing-policies.html). No write action was attempted to prove live authorization.

The administrative identity was used only for read-only inventory. This audit does not treat the absence of a checked-in administrative workflow as proof that an external operator or credentials stored outside AWS cannot publish.

## Recent ECR activity evidence

The ECR CloudTrail query covered 2026-06-01 through the audit time and matched the exact request parameter <code>repositoryName = portfolio</code>. It returned only read operations:

- <code>DescribeImages</code>;
- <code>GetLifecyclePolicy</code>; and
- <code>GetRepositoryPolicy</code>.

It returned no <code>PutImage</code>, layer-upload, <code>BatchGetImage</code>, or <code>GetDownloadUrlForLayer</code> event for the legacy repository in the available window. This agrees with the image metadata, whose last push and last recorded pull are both on 2026-04-28.

AWS documents that ECR image pushes generate layer-upload and <code>PutImage</code> events, and pulls generate <code>GetDownloadUrlForLayer</code> and <code>BatchGetImage</code> events. See [Logging Amazon ECR actions with AWS CloudTrail](https://docs.aws.amazon.com/AmazonECR/latest/userguide/logging-using-cloudtrail.html).

No CloudTrail trail or CloudTrail Lake event data store was found within this Region-scoped audit. CloudTrail event history is Region-scoped and limited to the recent 90-day management-event window. See [Working with CloudTrail event history](https://docs.aws.amazon.com/awscloudtrail/latest/userguide/view-cloudtrail-events.html). Therefore:

- activity before the available window is not queryable here;
- the successful April push and pull are outside the retained event-history window;
- absence of a recent event is not proof of lifetime non-use; and
- this audit did not locate a durable audit log in scope to search for older external publishers or consumers.

## Explicit gaps

The audit does not prove any of the following:

1. That no workload outside <code>us-west-2</code> or outside the expected account has credentials capable of accessing the regional URI. Other Regions and accounts were outside the authorization.
2. That no laptop, on-premises host, external CI service, deleted IAM principal, cached ECR token, or static credential has ever pulled or pushed the image.
3. That the running EC2 instance has no manually installed credential or cached image. Its AWS configuration was inspected, not its filesystem.
4. That no pull occurred before the available CloudTrail window. The repository's own last-recorded-pull field is stronger evidence for the current retained images, but it is not a complete request ledger.
5. That IAM simulation exactly predicts a live request. No write was attempted.
6. That removing the HCL would produce an immediately applicable deletion. The live repository is nonempty and current configuration has <code>force_delete = false</code>; this audit did not create a plan or test a delete.
7. That a future operator cannot recreate a publisher path from Git history or manual AWS/Docker commands.

## Cleanup gate

Deletion of <code>portfolio</code> is prohibited in this pull request. If repository retirement is later approved, it should be a separate, reviewed operation with all of these conditions:

1. Re-run the read-only App Runner, Lambda version, ECS task-definition, EKS, Batch, CodeBuild, SageMaker, Lightsail, ECR last-pull, IAM, and CloudTrail checks immediately before planning.
2. Decide whether the six legacy image records require export or retention. Any image deletion is destructive and needs explicit approval.
3. Remove the legacy repository, lifecycle policy, and ECR output from the root HCL together with the orphaned <code>_check-aws</code>, <code>_ecr-ensure-repo</code>, and <code>_ecr-docker-push</code> helpers and stale login/push guidance.
4. Create and review a full OpenTofu plan against the exact state lineage and lock. The plan must preserve DynamoDB, shared IAM, SSM, every <code>infra/lambda/</code> resource, and <code>portfolio-lambda-releases</code>.
5. Obtain separate approval for the exact destructive image/repository action and saved-plan checksum.
6. After apply, verify the root state no longer owns the repository, the legacy ARN is absent, the replacement repository and Lambda digest are unchanged, and repository tests/docs have no remaining live caller.

Until those gates are met, the supported conclusion is:

> No identified live managed consumer of <code>portfolio</code> remains in the authorized Region, but deletion is not authorized and universal non-use has not been proven.
