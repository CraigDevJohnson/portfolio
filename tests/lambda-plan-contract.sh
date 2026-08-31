#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
checker="$repo_root/scripts/check-lambda-plan.sh"
ci_roles_checker="$repo_root/scripts/check-ci-roles-plan.sh"
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM
release_repository=180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases
release_digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
release_image="$release_repository@$release_digest"
release_source_sha=0123456789abcdef0123456789abcdef01234567
release_tag="git-$release_source_sha"
release_tagged_image="$release_repository:$release_tag"
previous_release_digest=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
previous_release_image="$release_repository@$previous_release_digest"
ci_roles_state_root=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/ci-roles
ci_roles_lock_uri="$ci_roles_state_root/terraform.tfstate.tflock"
artifact_state_root=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/artifacts
artifact_lock_uri="$artifact_state_root/terraform.tfstate.tflock"
dev_state_root=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev
dev_lock_uri="$dev_state_root/terraform.tfstate.tflock"
prod_state_root=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/prod
prod_lock_uri="$prod_state_root/terraform.tfstate.tflock"

pass_count=0

pass() {
  pass_count=$((pass_count + 1))
  printf 'PASS: %s\n' "$1"
}

expect_pass() {
  name=$1
  shift
  if "$@" > "$tmp_dir/output" 2>&1; then
    pass "$name"
  else
    printf 'FAIL: %s\n' "$name" >&2
    cat "$tmp_dir/output" >&2
    exit 1
  fi
}

expect_fail() {
  name=$1
  shift
  if "$@" > "$tmp_dir/output" 2>&1; then
    printf 'FAIL: %s unexpectedly passed\n' "$name" >&2
    exit 1
  fi
  pass "$name"
}

make_artifact_plan() {
  jq -n '{
    resource_changes: [
      {
        address: "aws_ecr_repository.lambda_releases",
        type: "aws_ecr_repository",
        name: "lambda_releases",
        change: {actions: ["create"], after: {
          name: "portfolio-lambda-releases",
          image_tag_mutability: "IMMUTABLE",
          force_delete: false,
          image_scanning_configuration: [{scan_on_push: true}],
          encryption_configuration: [{encryption_type: "AES256"}]
        }, after_sensitive: {}}
      },
      {
        address: "aws_ecr_lifecycle_policy.lambda_releases",
        type: "aws_ecr_lifecycle_policy",
        name: "lambda_releases",
        change: {actions: ["create"], after: {
          repository: "portfolio-lambda-releases",
          policy: ({rules: [{
            rulePriority: 1,
            description: "Expire untagged images after 30 days",
            selection: {
              tagStatus: "untagged",
              countType: "sinceImagePushed",
              countUnit: "days",
              countNumber: 30
            },
            action: {type: "expire"}
          }]} | tojson)
        }, after_sensitive: {}}
      },
      {
        address: "aws_ecr_repository_policy.lambda_releases",
        type: "aws_ecr_repository_policy",
        name: "lambda_releases",
        change: {actions: ["create"], after: {
          repository: "portfolio-lambda-releases",
          policy: ({
            Version: "2012-10-17",
            Statement: [{
              Sid: "LambdaPull",
              Effect: "Allow",
              Principal: {Service: "lambda.amazonaws.com"},
              Action: ["ecr:BatchGetImage", "ecr:GetDownloadUrlForLayer"],
              Condition: {
                StringEquals: {"aws:SourceAccount": "180294223248"},
                ArnLike: {
                  "aws:SourceArn": "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-*"
                }
              }
            }]
          } | tojson)
        }, after_sensitive: {}}
      }
    ]
  }' > "$1"
}

make_ci_roles_plan() {
  jq -n '
    def change($after; $after_unknown): {
      actions: ["create"],
      before: null,
      after: $after,
      after_unknown: $after_unknown,
      before_sensitive: false,
      after_sensitive: {}
    };
    def managed($address; $type; $name; $index; $after; $after_unknown):
      {
        mode: "managed",
        address: $address,
        type: $type,
        name: $name,
        provider_name: "registry.opentofu.org/hashicorp/aws",
        change: change($after; $after_unknown)
      } + (if $index == null then {} else {index: $index} end);
    def trust($subject): ({
      Version: "2012-10-17",
      Statement: [{
        Effect: "Allow",
        Action: "sts:AssumeRoleWithWebIdentity",
        Principal: {
          Federated: "arn:aws:iam::180294223248:oidc-provider/token.actions.githubusercontent.com"
        },
        Condition: {
          StringEquals: {
            "token.actions.githubusercontent.com:aud": "sts.amazonaws.com",
            "token.actions.githubusercontent.com:sub": $subject
          }
        }
      }]
    } | tojson);
    def role($address; $index; $name; $subject):
      managed(
        $address;
        "aws_iam_role";
        "ci";
        $index;
        {
          arn: null,
          assume_role_policy: trust($subject),
          create_date: null,
          description: null,
          force_detach_policies: false,
          id: null,
          inline_policy: [],
          managed_policy_arns: null,
          max_session_duration: 3600,
          name: $name,
          name_prefix: null,
          path: "/",
          permissions_boundary: null,
          tags: {
            ManagedBy: "opentofu",
            Project: "portfolio",
            Purpose: "github-release"
          },
          tags_all: {
            ManagedBy: "opentofu",
            Project: "portfolio",
            Purpose: "github-release"
          },
          unique_id: null
        };
        {arn: true, create_date: true, id: true, managed_policy_arns: true, unique_id: true}
      );
    def environment_statements($environment; $function_name; $state_key): [
      {
        Sid: "CallerIdentity",
        Effect: "Allow",
        Action: ["sts:GetCallerIdentity"],
        Resource: "*"
      },
      {
        Sid: "StateBucketMetadata",
        Effect: "Allow",
        Action: ["s3:GetBucketLocation", "s3:GetBucketVersioning"],
        Resource: "arn:aws:s3:::portfolio-tofu-state-180294223248"
      },
      {
        Sid: "StatePrefix",
        Effect: "Allow",
        Action: ["s3:ListBucket"],
        Resource: "arn:aws:s3:::portfolio-tofu-state-180294223248",
        Condition: {StringLike: {"s3:prefix": [($state_key + "*")]}}
      },
      {
        Sid: "StateRead",
        Effect: "Allow",
        Action: ["s3:GetObject"],
        Resource: ("arn:aws:s3:::portfolio-tofu-state-180294223248/" + $state_key)
      },
      {
        Sid: "StateLock",
        Effect: "Allow",
        Action: ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
        Resource: ("arn:aws:s3:::portfolio-tofu-state-180294223248/" + $state_key + ".tflock")
      },
      {
        Sid: "ReleaseImageRead",
        Effect: "Allow",
        Action: ["ecr:BatchGetImage", "ecr:DescribeImages", "ecr:GetDownloadUrlForLayer"],
        Resource: "arn:aws:ecr:us-west-2:180294223248:repository/portfolio-lambda-releases"
      },
      {
        Sid: "ExecutionRoleRead",
        Effect: "Allow",
        Action: [
          "iam:GetRole",
          "iam:GetRolePolicy",
          "iam:ListAttachedRolePolicies",
          "iam:ListRolePolicies",
          "iam:ListRoleTags"
        ],
        Resource: ("arn:aws:iam::180294223248:role/" + $function_name + "-execution")
      },
      {
        Sid: "LambdaRead",
        Effect: "Allow",
        Action: [
          "lambda:GetAlias",
          "lambda:GetFunction",
          "lambda:GetFunctionCodeSigningConfig",
          "lambda:GetFunctionConcurrency",
          "lambda:GetFunctionConfiguration",
          "lambda:GetPolicy",
          "lambda:GetRuntimeManagementConfig",
          "lambda:ListTags",
          "lambda:ListVersionsByFunction"
        ],
        Resource: [
          ("arn:aws:lambda:us-west-2:180294223248:function:" + $function_name),
          ("arn:aws:lambda:us-west-2:180294223248:function:" + $function_name + ":*")
        ]
      },
      {
        Sid: "ApiGatewayRead",
        Effect: "Allow",
        Action: ["apigateway:GET"],
        Resource: ["arn:aws:apigateway:us-west-2::/apis*", "arn:aws:apigateway:us-west-2::/domainnames*"]
      },
      {
        Sid: "LogGroupRead",
        Effect: "Allow",
        Action: ["logs:ListTagsForResource"],
        Resource: [
          ("arn:aws:logs:us-west-2:180294223248:log-group:/aws/apigateway/" + $function_name + "/access"),
          ("arn:aws:logs:us-west-2:180294223248:log-group:/aws/apigateway/" + $function_name + "/access:*"),
          ("arn:aws:logs:us-west-2:180294223248:log-group:/aws/lambda/" + $function_name),
          ("arn:aws:logs:us-west-2:180294223248:log-group:/aws/lambda/" + $function_name + ":*")
        ]
      },
      {
        Sid: "LogGroupList",
        Effect: "Allow",
        Action: ["logs:DescribeLogGroups"],
        Resource: "*",
        Condition: {StringEquals: {"aws:RequestedRegion": "us-west-2"}}
      },
      {
        Sid: "TableRead",
        Effect: "Allow",
        Action: [
          "dynamodb:DescribeContinuousBackups",
          "dynamodb:DescribeTable",
          "dynamodb:DescribeTimeToLive",
          "dynamodb:ListTagsOfResource"
        ],
        Resource: [
          ("arn:aws:dynamodb:us-west-2:180294223248:table/" + $function_name + "-google-connections"),
          ("arn:aws:dynamodb:us-west-2:180294223248:table/" + $function_name + "-soccer-sessions")
        ]
      },
      {
        Sid: "KmsAliasList",
        Effect: "Allow",
        Action: ["kms:ListAliases"],
        Resource: "*",
        Condition: {StringEquals: {"aws:RequestedRegion": "us-west-2"}}
      },
      {
        Sid: "KmsSsmKeyRead",
        Effect: "Allow",
        Action: ["kms:DescribeKey"],
        Resource: "arn:aws:kms:us-west-2:180294223248:key/*",
        Condition: {"ForAnyValue:StringEquals": {"kms:ResourceAliases": "alias/aws/ssm"}}
      },
      {
        Sid: "CertificateRead",
        Effect: "Allow",
        Action: ["acm:DescribeCertificate", "acm:ListTagsForCertificate"],
        Resource: "arn:aws:acm:us-west-2:180294223248:certificate/*",
        Condition: {StringEquals: {
          "aws:RequestedRegion": "us-west-2",
          "aws:ResourceTag/Environment": $environment,
          "aws:ResourceTag/ManagedBy": "opentofu",
          "aws:ResourceTag/Platform": "lambda-http-api",
          "aws:ResourceTag/Project": "portfolio"
        }}
      },
      {
        Sid: "AlarmRead",
        Effect: "Allow",
        Action: ["cloudwatch:DescribeAlarms", "cloudwatch:ListTagsForResource"],
        Resource: [
          ("arn:aws:cloudwatch:us-west-2:180294223248:alarm:" + $function_name + "-api-5xx"),
          ("arn:aws:cloudwatch:us-west-2:180294223248:alarm:" + $function_name + "-api-latency"),
          ("arn:aws:cloudwatch:us-west-2:180294223248:alarm:" + $function_name + "-lambda-duration"),
          ("arn:aws:cloudwatch:us-west-2:180294223248:alarm:" + $function_name + "-lambda-errors"),
          ("arn:aws:cloudwatch:us-west-2:180294223248:alarm:" + $function_name + "-lambda-throttles")
        ]
      }
    ];
    def development_mutations: [
      {
        Sid: "DevelopmentStateWrite",
        Effect: "Allow",
        Action: ["s3:PutObject", "s3:DeleteObject"],
        Resource: "arn:aws:s3:::portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate"
      },
      {
        Sid: "DevelopmentReleaseWrite",
        Effect: "Allow",
        Action: ["lambda:PublishVersion", "lambda:UpdateAlias", "lambda:UpdateFunctionCode"],
        Resource: [
          "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-dev",
          "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-dev:live"
        ],
        Condition: {StringEquals: {
          "aws:ResourceTag/Environment": "dev",
          "aws:ResourceTag/ManagedBy": "opentofu",
          "aws:ResourceTag/Platform": "lambda-http-api",
          "aws:ResourceTag/Project": "portfolio"
        }}
      }
    ];
    def environment_policy($environment; $function_name; $state_key; $mutable): ({
      Version: "2012-10-17",
      Statement: (
        environment_statements($environment; $function_name; $state_key) +
        (if $mutable then development_mutations else [] end)
      )
    } | tojson);
    def release_policy: ({
      Version: "2012-10-17",
      Statement: [
        {
          Effect: "Allow",
          Action: ["ecr:GetAuthorizationToken"],
          Resource: "*",
          Condition: {StringEquals: {"aws:RequestedRegion": "us-west-2"}}
        },
        {
          Effect: "Allow",
          Action: [
            "ecr:BatchCheckLayerAvailability",
            "ecr:CompleteLayerUpload",
            "ecr:DescribeImageScanFindings",
            "ecr:DescribeImages",
            "ecr:DescribeRepositories",
            "ecr:GetDownloadUrlForLayer",
            "ecr:InitiateLayerUpload",
            "ecr:ListImages",
            "ecr:PutImage",
            "ecr:UploadLayerPart"
          ],
          Resource: "arn:aws:ecr:us-west-2:180294223248:repository/portfolio-lambda-releases"
        }
      ]
    } | tojson);
    def role_policy($address; $resource_name; $index; $policy_name; $role_name; $policy):
      managed(
        $address;
        "aws_iam_role_policy";
        $resource_name;
        $index;
        {id: null, name: $policy_name, name_prefix: null, policy: $policy, role: $role_name};
        {id: true}
      );
    {
      format_version: "1.2",
      terraform_version: "1.12.6",
      resource_changes: [
        role(
          "aws_iam_role.ci[\"release\"]";
          "release";
          "portfolio-release-builder-ci";
          "repo:CraigDevJohnson/portfolio:ref:refs/heads/main"
        ),
        role(
          "aws_iam_role.ci[\"dev\"]";
          "dev";
          "portfolio-development-deployer-ci";
          "repo:CraigDevJohnson/portfolio:environment:development"
        ),
        role(
          "aws_iam_role.ci[\"prod\"]";
          "prod";
          "portfolio-production-planner-ci";
          "repo:CraigDevJohnson/portfolio:environment:production-plan"
        ),
        role_policy(
          "aws_iam_role_policy.release";
          "release";
          null;
          "portfolio-release-builder";
          "portfolio-release-builder-ci";
          release_policy
        ),
        role_policy(
          "aws_iam_role_policy.environment[\"dev\"]";
          "environment";
          "dev";
          "portfolio-development-runtime-release";
          "portfolio-development-deployer-ci";
          environment_policy(
            "dev";
            "portfolio-lambda-dev";
            "portfolio-lambda-http-api/dev/terraform.tfstate";
            true
          )
        ),
        role_policy(
          "aws_iam_role_policy.environment[\"prod\"]";
          "environment";
          "prod";
          "portfolio-production-read-only-plan";
          "portfolio-production-planner-ci";
          environment_policy(
            "prod";
            "portfolio-lambda-prod";
            "portfolio-lambda-http-api/prod/terraform.tfstate";
            false
          )
        )
      ],
      output_changes: {},
      errored: false,
      configuration: {
        provider_config: {
          aws: {
            name: "aws",
            full_name: "registry.opentofu.org/hashicorp/aws",
            version_constraint: "6.38.0",
            expressions: {
              allowed_account_ids: {constant_value: ["180294223248"]},
              profile: {constant_value: "portfolio-ci-roles-administrator"},
              region: {references: ["local.region"]}
            }
          }
        },
        root_module: {
          resources: [
            {
              address: "aws_iam_role.ci",
              mode: "managed",
              type: "aws_iam_role",
              name: "ci",
              provider_config_key: "aws",
              schema_version: 0,
              for_each_expression: {references: ["local.roles"]},
              expressions: {
                assume_role_policy: {references: ["each.value.trust", "each.value"]},
                max_session_duration: {constant_value: 3600},
                name: {references: ["each.value.name", "each.value"]},
                tags: {constant_value: {
                  ManagedBy: "opentofu",
                  Project: "portfolio",
                  Purpose: "github-release"
                }}
              }
            },
            {
              address: "aws_iam_role_policy.environment",
              mode: "managed",
              type: "aws_iam_role_policy",
              name: "environment",
              provider_config_key: "aws",
              schema_version: 0,
              depends_on: ["aws_iam_role.ci"],
              for_each_expression: {references: ["local.environment_configuration"]},
              expressions: {
                name: {references: ["each.value.policy_name", "each.value"]},
                policy: {references: ["local.environment_policies", "each.key"]},
                role: {references: ["each.value.role_name", "each.value"]}
              }
            },
            {
              address: "aws_iam_role_policy.release",
              mode: "managed",
              type: "aws_iam_role_policy",
              name: "release",
              provider_config_key: "aws",
              schema_version: 0,
              depends_on: ["aws_iam_role.ci"],
              expressions: {
                name: {constant_value: "portfolio-release-builder"},
                policy: {references: ["local.region", "local.ecr_repository_arn"]},
                role: {references: [
                  "local.roles.release.name",
                  "local.roles.release",
                  "local.roles"
                ]}
              }
            },
            {
              address: "data.aws_iam_policy_document.environment_trust",
              mode: "data",
              type: "aws_iam_policy_document",
              name: "environment_trust",
              provider_config_key: "aws",
              schema_version: 0,
              for_each_expression: {references: ["local.environment_configuration"]},
              expressions: {
                statement: [{
                  actions: {constant_value: ["sts:AssumeRoleWithWebIdentity"]},
                  condition: [{
                    test: {constant_value: "StringEquals"},
                    values: {constant_value: ["sts.amazonaws.com"]},
                    variable: {constant_value: "token.actions.githubusercontent.com:aud"}
                  }, {
                    test: {constant_value: "StringEquals"},
                    values: {references: ["each.value.github_environment", "each.value"]},
                    variable: {constant_value: "token.actions.githubusercontent.com:sub"}
                  }],
                  principals: [{
                    identifiers: {references: ["local.github_oidc_provider_arn"]},
                    type: {constant_value: "Federated"}
                  }]
                }]
              }
            },
            {
              address: "data.aws_iam_policy_document.release_trust",
              mode: "data",
              type: "aws_iam_policy_document",
              name: "release_trust",
              provider_config_key: "aws",
              schema_version: 0,
              expressions: {
                statement: [{
                  actions: {constant_value: ["sts:AssumeRoleWithWebIdentity"]},
                  condition: [{
                    test: {constant_value: "StringEquals"},
                    values: {constant_value: ["sts.amazonaws.com"]},
                    variable: {constant_value: "token.actions.githubusercontent.com:aud"}
                  }, {
                    test: {constant_value: "StringEquals"},
                    values: {constant_value: ["repo:CraigDevJohnson/portfolio:ref:refs/heads/main"]},
                    variable: {constant_value: "token.actions.githubusercontent.com:sub"}
                  }],
                  principals: [{
                    identifiers: {references: ["local.github_oidc_provider_arn"]},
                    type: {constant_value: "Federated"}
                  }]
                }]
              }
            }
          ]
        }
      }
    }
  ' > "$1"
}

make_environment_plan() {
  output=$1
  environment=$2
  prefix="portfolio-lambda-$environment"
  image_uri=$release_image
  if [ "$environment" = prod ]; then
    protection=true
    retention=90
    reserved_concurrency=10
    alarm_actions='["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts"]'
  else
    protection=false
    retention=14
    reserved_concurrency=-1
    alarm_actions='[]'
  fi
  jq -n \
    --arg environment "$environment" \
    --arg prefix "$prefix" \
    --arg image "$image_uri" \
    --arg boundary "arn:aws:iam::180294223248:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary" \
    --argjson protection "$protection" \
    --argjson retention "$retention" \
    --argjson reserved_concurrency "$reserved_concurrency" \
    --argjson alarm_actions "$alarm_actions" '
    def change($after): {
      actions: ["create"],
      before: null,
      after: $after,
      after_unknown: {},
      before_sensitive: false,
      after_sensitive: {}
    };
    def resource($address; $type; $name; $after): {
      mode: "managed",
      address: $address,
      type: $type,
      name: $name,
      change: change($after)
    };
    def policy_statement($actions; $resources): {
      actions: $actions,
      condition: [],
      effect: null,
      not_actions: null,
      not_principals: [],
      not_resources: null,
      principals: [],
      resources: $resources,
      sid: null
    };
    def policy_statements: [
      policy_statement(
        ["dynamodb:DeleteItem", "dynamodb:GetItem", "dynamodb:PutItem"];
        ["arn:aws:dynamodb:us-west-2:180294223248:table/" + $prefix + "-google-connections"]
      ),
      policy_statement(
        ["dynamodb:PutItem"];
        ["arn:aws:dynamodb:us-west-2:180294223248:table/" + $prefix + "-soccer-sessions"]
      ),
      policy_statement(["ssm:GetParameters"]; [
        "arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/" + $environment + "/CLIENT_ID_KEY",
        "arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/" + $environment + "/CLIENT_SECRET_KEY",
        "arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/" + $environment + "/LPS_SESSION_KEY"
      ]),
      policy_statement(
        ["kms:Decrypt"];
        ["arn:aws:kms:us-west-2:180294223248:key/00000000-0000-0000-0000-000000000000"]
      ),
      policy_statement(
        ["logs:CreateLogStream", "logs:PutLogEvents"];
        ["arn:aws:logs:us-west-2:180294223248:log-group:/aws/lambda/" + $prefix + ":*"]
      )
    ];
    def policy_statement_expression($actions; $references): {
      actions: {constant_value: $actions},
      resources: {references: $references}
    };
    def policy_statement_expressions: [
      policy_statement_expression(
        ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:DeleteItem"];
        ["aws_dynamodb_table.google_connections.arn", "aws_dynamodb_table.google_connections"]
      ),
      policy_statement_expression(
        ["dynamodb:PutItem"];
        ["aws_dynamodb_table.soccer_sessions.arn", "aws_dynamodb_table.soccer_sessions"]
      ),
      policy_statement_expression(
        ["ssm:GetParameters"];
        [
          "local.ssm_paths",
          "data.aws_partition.current.partition",
          "data.aws_partition.current",
          "var.aws_region",
          "data.aws_caller_identity.current.account_id",
          "data.aws_caller_identity.current"
        ]
      ),
      policy_statement_expression(
        ["kms:Decrypt"];
        ["data.aws_kms_alias.ssm.target_key_arn", "data.aws_kms_alias.ssm"]
      ),
      policy_statement_expression(
        ["logs:CreateLogStream", "logs:PutLogEvents"];
        ["aws_cloudwatch_log_group.lambda.arn", "aws_cloudwatch_log_group.lambda"]
      )
    ];
    def lambda_variables: {
      CLIENT_ID_KEY: ("/portfolio/lambda/" + $environment + "/CLIENT_ID_KEY"),
      CLIENT_SECRET_KEY: ("/portfolio/lambda/" + $environment + "/CLIENT_SECRET_KEY"),
      GOOGLE_CONNECTION_TABLE_NAME: ($prefix + "-google-connections"),
      LOG_ADD_SOURCE: "false",
      LOG_FORMAT: "json",
      LOG_LEVEL: "info",
      LPS_SESSION_KEY: ("/portfolio/lambda/" + $environment + "/LPS_SESSION_KEY"),
      SOCCER_SESSION_TABLE_NAME: ($prefix + "-soccer-sessions")
    };
    def alarm($short; $metric; $threshold; $statistic):
      resource("module.service.aws_cloudwatch_metric_alarm." + $short; "aws_cloudwatch_metric_alarm"; $short; {
        alarm_name: ($prefix + "-" + ($short | gsub("_"; "-"))),
        metric_name: $metric,
        period: 300,
        evaluation_periods: 1,
        threshold: $threshold,
        treat_missing_data: "notBreaching",
        alarm_actions: $alarm_actions
      } + (if $statistic == "p95" then {extended_statistic: "p95"} else {statistic: $statistic} end));
    def alarm_configuration($short): {
      address: ("aws_cloudwatch_metric_alarm." + $short),
      mode: "managed",
      type: "aws_cloudwatch_metric_alarm",
      name: $short,
      expressions: {alarm_actions: {references: ["var.alarm_action_arns"]}}
    };
    def configuration_resource($address; $mode; $type; $name): {
      address: $address,
      mode: $mode,
      type: $type,
      name: $name,
      expressions: {}
    };
    def simple_configuration_resources:
      [
        ["aws_acm_certificate.custom", "managed", "aws_acm_certificate", "custom"],
        ["aws_acm_certificate_validation.custom", "managed", "aws_acm_certificate_validation", "custom"],
        ["aws_apigatewayv2_api.app", "managed", "aws_apigatewayv2_api", "app"],
        ["aws_apigatewayv2_api_mapping.custom", "managed", "aws_apigatewayv2_api_mapping", "custom"],
        ["aws_apigatewayv2_domain_name.custom", "managed", "aws_apigatewayv2_domain_name", "custom"],
        ["aws_apigatewayv2_integration.lambda", "managed", "aws_apigatewayv2_integration", "lambda"],
        ["aws_apigatewayv2_route.default", "managed", "aws_apigatewayv2_route", "default"],
        ["aws_apigatewayv2_stage.default", "managed", "aws_apigatewayv2_stage", "default"],
        ["aws_cloudwatch_log_group.api_access", "managed", "aws_cloudwatch_log_group", "api_access"],
        ["aws_cloudwatch_log_group.lambda", "managed", "aws_cloudwatch_log_group", "lambda"],
        ["aws_dynamodb_table.google_connections", "managed", "aws_dynamodb_table", "google_connections"],
        ["aws_dynamodb_table.soccer_sessions", "managed", "aws_dynamodb_table", "soccer_sessions"],
        ["aws_iam_role.lambda", "managed", "aws_iam_role", "lambda"],
        ["aws_lambda_alias.live", "managed", "aws_lambda_alias", "live"],
        ["aws_lambda_function.app", "managed", "aws_lambda_function", "app"],
        ["aws_lambda_permission.api", "managed", "aws_lambda_permission", "api"],
        ["data.aws_caller_identity.current", "data", "aws_caller_identity", "current"],
        [
          "data.aws_iam_policy_document.lambda_assume_role",
          "data",
          "aws_iam_policy_document",
          "lambda_assume_role"
        ],
        ["data.aws_partition.current", "data", "aws_partition", "current"]
      ] |
      map(configuration_resource(.[0]; .[1]; .[2]; .[3]));
    {
      variables: {
        alarm_action_arns: {value: $alarm_actions},
        live_version_override: {value: null}
      },
      resource_changes: [
        resource(
          "module.service.aws_iam_role.lambda";
          "aws_iam_role";
          "lambda";
          {name: ($prefix + "-execution"), permissions_boundary: $boundary}
        ),
        (resource(
          "module.service.aws_iam_role_policy.lambda";
          "aws_iam_role_policy";
          "lambda";
          {name: ($prefix + "-runtime")}
        ) | .change.after_unknown = {id: true, name_prefix: true, policy: true, role: true}),
        resource(
          "module.service.aws_lambda_function.app";
          "aws_lambda_function";
          "app";
          {
            function_name: $prefix,
            image_uri: $image,
            reserved_concurrent_executions: $reserved_concurrency,
            environment: [{variables: lambda_variables}]
          }
        ),
        resource("module.service.aws_apigatewayv2_api.app"; "aws_apigatewayv2_api"; "app"; {name: ($prefix + "-http")}),
        resource(
          "module.service.aws_cloudwatch_log_group.lambda";
          "aws_cloudwatch_log_group";
          "lambda";
          {
            name: ("/aws/lambda/" + $prefix),
            arn: ("arn:aws:logs:us-west-2:180294223248:log-group:/aws/lambda/" + $prefix),
            retention_in_days: $retention
          }
        ),
        resource(
          "module.service.aws_cloudwatch_log_group.api_access";
          "aws_cloudwatch_log_group";
          "api_access";
          {name: ("/aws/apigateway/" + $prefix + "/access"), retention_in_days: $retention}
        ),
        resource(
          "module.service.aws_dynamodb_table.google_connections";
          "aws_dynamodb_table";
          "google_connections";
          {
            name: ($prefix + "-google-connections"),
            arn: ("arn:aws:dynamodb:us-west-2:180294223248:table/" + $prefix + "-google-connections"),
            deletion_protection_enabled: $protection,
            point_in_time_recovery: [{enabled: $protection}]
          }
        ),
        resource(
          "module.service.aws_dynamodb_table.soccer_sessions";
          "aws_dynamodb_table";
          "soccer_sessions";
          {
            name: ($prefix + "-soccer-sessions"),
            arn: ("arn:aws:dynamodb:us-west-2:180294223248:table/" + $prefix + "-soccer-sessions"),
            deletion_protection_enabled: $protection,
            point_in_time_recovery: [{enabled: $protection}]
          }
        ),
        alarm("lambda_errors"; "Errors"; 1; "Sum"),
        alarm("lambda_throttles"; "Throttles"; 1; "Sum"),
        alarm("lambda_duration"; "Duration"; 24000; "p95"),
        alarm("api_5xx"; "5xx"; 1; "Sum"),
        alarm("api_latency"; "Latency"; 25000; "p95"),
        {
          mode: "data",
          address: "module.service.data.aws_iam_policy_document.lambda",
          type: "aws_iam_policy_document",
          name: "lambda",
          change: {
            actions: ["read"],
            before: null,
            after: {
              override_json: null,
              override_policy_documents: null,
              policy_id: null,
              source_json: null,
              source_policy_documents: null,
              statement: policy_statements,
              version: null
            },
            after_unknown: {id: true, json: true, minified_json: true},
            before_sensitive: false,
            after_sensitive: {}
          }
        }
      ],
      configuration: {
        root_module: {
          module_calls: {
            service: {
              expressions: {alarm_action_arns: {references: ["var.alarm_action_arns"]}},
              module: {
                resources: (simple_configuration_resources + [
                  {
                    address: "aws_iam_role_policy.lambda",
                    mode: "managed",
                    type: "aws_iam_role_policy",
                    name: "lambda",
                    expressions: {
                      policy: {
                        references: [
                          "data.aws_iam_policy_document.lambda.json",
                          "data.aws_iam_policy_document.lambda"
                        ]
                      }
                    }
                  }, {
                    address: "data.aws_iam_policy_document.lambda",
                    mode: "data",
                    type: "aws_iam_policy_document",
                    name: "lambda",
                    expressions: {statement: policy_statement_expressions}
                  }, {
                    address: "data.aws_kms_alias.ssm",
                    mode: "data",
                    type: "aws_kms_alias",
                    name: "ssm",
                    expressions: {name: {constant_value: "alias/aws/ssm"}}
                  },
                  alarm_configuration("lambda_errors"),
                  alarm_configuration("lambda_throttles"),
                  alarm_configuration("lambda_duration"),
                  alarm_configuration("api_5xx"),
                  alarm_configuration("api_latency")
                ])
              }
            }
          }
        }
      }
    }' > "$output"
}

run_check() {
  plan=$1
  environment=$2
  if [ "$environment" = artifacts ]; then
    prefix=portfolio-lambda-releases
    actions='[]'
  else
    prefix="portfolio-lambda-$environment"
    if [ "$environment" = prod ]; then
      actions='["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts"]'
    else
      actions='[]'
    fi
  fi
  PLAN_JSON="$plan" \
    ENVIRONMENT="$environment" \
    NAME_PREFIX="$prefix" \
    IMAGE_URI="$release_image" \
    EXPECTED_ALARM_ACTIONS_JSON="$actions" \
    sh "$checker"
}

run_ci_roles_check() {
  PLAN_JSON="$1" sh "$ci_roles_checker"
}

run_maintenance_check() {
  plan=$1
  AUTOMATED_RELEASE=true \
    PLAN_JSON="$plan" \
    ENVIRONMENT=dev \
    NAME_PREFIX=portfolio-lambda-dev \
    IMAGE_URI="$release_image" \
    EXPECTED_ALARM_ACTIONS_JSON='[]' \
    sh "$checker"
}

run_rollback_check() {
  plan=$1
  prior_version=$2
  AUTOMATED_RELEASE=rollback \
    PRIOR_VERSION="$prior_version" \
    PLAN_JSON="$plan" \
    ENVIRONMENT=dev \
    NAME_PREFIX=portfolio-lambda-dev \
    IMAGE_URI="$release_image" \
    EXPECTED_ALARM_ACTIONS_JSON='[]' \
    sh "$checker"
}

artifact_plan="$tmp_dir/artifact.json"
ci_roles_plan="$tmp_dir/ci-roles.json"
dev_plan="$tmp_dir/dev.json"
prod_plan="$tmp_dir/prod.json"
prod_partial_plan="$tmp_dir/prod-partial.json"
make_artifact_plan "$artifact_plan"
make_ci_roles_plan "$ci_roles_plan"
make_environment_plan "$dev_plan" dev
make_environment_plan "$prod_partial_plan" prod
jq '
  def resource($address; $type; $name; $after): {
    mode: "managed",
    address: $address,
    type: $type,
    name: $name,
    change: {
      actions: ["create"],
      before: null,
      after: $after,
      after_unknown: {},
      before_sensitive: false,
      after_sensitive: {}
    }
  };
  .resource_changes += ([
    ["module.service.aws_apigatewayv2_integration.lambda", "aws_apigatewayv2_integration", "lambda"],
    ["module.service.aws_apigatewayv2_route.default", "aws_apigatewayv2_route", "default"],
    ["module.service.aws_apigatewayv2_stage.default", "aws_apigatewayv2_stage", "default"],
    ["module.service.aws_lambda_alias.live", "aws_lambda_alias", "live"],
    ["module.service.aws_lambda_permission.api", "aws_lambda_permission", "api"]
  ] | map(resource(.[0]; .[1]; .[2]; {})))
' "$prod_partial_plan" > "$prod_plan"

prod_missing_integration_plan="$tmp_dir/prod-missing-integration.json"
jq 'del(.resource_changes[] |
  select(.address == "module.service.aws_apigatewayv2_integration.lambda"))' \
  "$prod_plan" > "$prod_missing_integration_plan"

prod_dormant_configuration_plan="$tmp_dir/prod-dormant-configuration.json"
jq '.configuration.root_module.module_calls.service.module.resources += [{
  address: "aws_iam_user.dormant",
  mode: "managed",
  type: "aws_iam_user",
  name: "dormant",
  count_expression: {constant_value: 0},
  expressions: {}
}]' "$prod_plan" > "$prod_dormant_configuration_plan"

prod_dormant_root_configuration_plan="$tmp_dir/prod-dormant-root-configuration.json"
jq '.configuration.root_module.resources = [{
  address: "aws_iam_user.dormant",
  mode: "managed",
  type: "aws_iam_user",
  name: "dormant",
  count_expression: {constant_value: 0},
  expressions: {}
}]' "$prod_plan" > "$prod_dormant_root_configuration_plan"

prod_dormant_sibling_module_plan="$tmp_dir/prod-dormant-sibling-module.json"
jq '.configuration.root_module.module_calls.dormant = {
  module: {resources: [{
    address: "aws_iam_user.dormant",
    mode: "managed",
    type: "aws_iam_user",
    name: "dormant",
    count_expression: {constant_value: 0},
    expressions: {}
  }]}
}' "$prod_plan" > "$prod_dormant_sibling_module_plan"

prod_dormant_nested_module_plan="$tmp_dir/prod-dormant-nested-module.json"
jq '.configuration.root_module.module_calls.service.module.module_calls.dormant = {
  module: {resources: [{
    address: "aws_iam_user.dormant",
    mode: "managed",
    type: "aws_iam_user",
    name: "dormant",
    count_expression: {constant_value: 0},
    expressions: {}
  }]}
}' "$prod_plan" > "$prod_dormant_nested_module_plan"

ci_roles_noop_plan="$tmp_dir/ci-roles-noop.json"
jq '
  .resource_changes |= map(
    (if .type == "aws_iam_role" then
      .change.after.arn = ("arn:aws:iam::180294223248:role/" + .change.after.name) |
      .change.after.create_date = "2026-08-30T15:00:00Z" |
      .change.after.id = .change.after.name |
      .change.after.managed_policy_arns = [] |
      .change.after.name_prefix = null |
      .change.after.unique_id = "AIDATESTUNIQUEID12345"
    else
      .change.after.id = (.change.after.role + ":" + .change.after.name) |
      .change.after.name_prefix = null
    end) |
    .change.before = .change.after |
    .change.actions = ["no-op"] |
    .change.after_unknown = {}
  )
' "$ci_roles_plan" > "$ci_roles_noop_plan"

ci_roles_refreshed_noop_plan="$tmp_dir/ci-roles-refreshed-noop.json"
jq '
  .resource_changes |= map(
    if .type == "aws_iam_role" then
      .change.before.description = "" |
      .change.before.name_prefix = "" |
      .change.before.permissions_boundary = "" |
      .change.after.description = "" |
      .change.after.name_prefix = "" |
      .change.after.permissions_boundary = ""
    elif .type == "aws_iam_role_policy" then
      .change.before.name_prefix = "" |
      .change.after.name_prefix = ""
    else
      .
    end
  )
' "$ci_roles_noop_plan" > "$ci_roles_refreshed_noop_plan"

ci_roles_description_noop_plan="$tmp_dir/ci-roles-description-noop.json"
jq '
  (.resource_changes[] |
    select(.address == "aws_iam_role.ci[\"release\"]") |
    .change) |= (
      .before.description = "unapproved" |
      .after.description = "unapproved"
    )
' "$ci_roles_refreshed_noop_plan" > "$ci_roles_description_noop_plan"

ci_roles_name_prefix_noop_plan="$tmp_dir/ci-roles-name-prefix-noop.json"
jq '
  (.resource_changes[] |
    select(.address == "aws_iam_role.ci[\"release\"]") |
    .change) |= (
      .before.name_prefix = "unapproved-" |
      .after.name_prefix = "unapproved-"
    )
' "$ci_roles_refreshed_noop_plan" > "$ci_roles_name_prefix_noop_plan"

ci_roles_boundary_noop_plan="$tmp_dir/ci-roles-boundary-noop.json"
jq '
  (.resource_changes[] |
    select(.address == "aws_iam_role.ci[\"release\"]") |
    .change) |= (
      .before.permissions_boundary = "arn:aws:iam::180294223248:policy/unapproved" |
      .after.permissions_boundary = "arn:aws:iam::180294223248:policy/unapproved"
    )
' "$ci_roles_refreshed_noop_plan" > "$ci_roles_boundary_noop_plan"

ci_roles_policy_name_prefix_noop_plan="$tmp_dir/ci-roles-policy-name-prefix-noop.json"
jq '
  (.resource_changes[] |
    select(.address == "aws_iam_role_policy.release") |
    .change) |= (
      .before.name_prefix = "unapproved-" |
      .after.name_prefix = "unapproved-"
    )
' "$ci_roles_refreshed_noop_plan" > "$ci_roles_policy_name_prefix_noop_plan"

ci_roles_narrow_update_plan="$tmp_dir/ci-roles-narrow-update.json"
jq '
  (.resource_changes[] | select(.address == "aws_iam_role_policy.release") | .change) |= (
    .actions = ["update"] |
    .before.policy |= (
      fromjson |
      (.Statement[] |
        select(.Resource == "arn:aws:ecr:us-west-2:180294223248:repository/portfolio-lambda-releases") |
        .Action) += ["ecr:DeleteRepository"] |
      tojson
    )
  )
' "$ci_roles_noop_plan" > "$ci_roles_narrow_update_plan"

dev_maintenance_plan="$tmp_dir/dev-maintenance.json"
jq --arg previous_image "$previous_release_image" '
  def noop_resource($address; $type; $name): {
    mode: "managed",
    address: $address,
    type: $type,
    name: $name,
    change: {
      actions: ["no-op"],
      before: {name: $name},
      after: {name: $name},
      after_unknown: {},
      before_sensitive: false,
      after_sensitive: {}
    }
  };
  (.resource_changes[] | select(.mode == "managed") | .change) |= (
    .before = .after |
    .actions = ["no-op"]
  ) |
  (.resource_changes[] | select(.address == "module.service.aws_lambda_function.app") | .change) |= (
    .actions = ["update"] |
    .before.image_uri = $previous_image |
    .before.timeout = 29 |
    .after.timeout = 29
  ) |
  .resource_changes += [
    {
      mode: "managed",
      address: "module.service.aws_lambda_alias.live",
      type: "aws_lambda_alias",
      name: "live",
      change: {
        actions: ["update"],
        before: {name: "live", function_name: "portfolio-lambda-dev", function_version: "7"},
        after: {name: "live", function_name: "portfolio-lambda-dev", function_version: null},
        after_unknown: {function_version: true},
        before_sensitive: false,
        after_sensitive: {}
      }
    },
    noop_resource(
      "module.service.aws_apigatewayv2_integration.lambda";
      "aws_apigatewayv2_integration";
      "lambda"
    ),
    noop_resource(
      "module.service.aws_apigatewayv2_route.default";
      "aws_apigatewayv2_route";
      "default"
    ),
    noop_resource(
      "module.service.aws_apigatewayv2_stage.default";
      "aws_apigatewayv2_stage";
      "default"
    ),
    noop_resource(
      "module.service.aws_lambda_permission.api";
      "aws_lambda_permission";
      "api"
    ),
    noop_resource(
      "module.service.aws_acm_certificate.custom[0]";
      "aws_acm_certificate";
      "custom"
    ),
    noop_resource(
      "module.service.aws_acm_certificate_validation.custom[0]";
      "aws_acm_certificate_validation";
      "custom"
    ),
    noop_resource(
      "module.service.aws_apigatewayv2_domain_name.custom[\"dev.craigdevjohnson.com\"]";
      "aws_apigatewayv2_domain_name";
      "custom"
    ),
    noop_resource(
      "module.service.aws_apigatewayv2_api_mapping.custom[\"dev.craigdevjohnson.com\"]";
      "aws_apigatewayv2_api_mapping";
      "custom"
    )
  ]
' "$dev_plan" > "$dev_maintenance_plan"

dev_known_policy_plan="$tmp_dir/dev-known-policy.json"
jq '
  (.resource_changes[] |
    select(.address == "module.service.data.aws_iam_policy_document.lambda") |
    .change.after.statement) as $statements |
  ({
    Version: "2012-10-17",
    Statement: [$statements[] | {
      Effect: "Allow",
      Action: (if (.actions | length) == 1 then .actions[0] else .actions end),
      Resource: (if (.resources | length) == 1 then .resources[0] else .resources end)
    }]
  } | tojson) as $policy |
  (.resource_changes[] |
    select(.address == "module.service.aws_iam_role_policy.lambda") |
    .change.after.policy) = $policy |
  del(
    .resource_changes[] |
    select(.address == "module.service.aws_iam_role_policy.lambda") |
    .change.after_unknown.policy
  )
' "$dev_plan" > "$dev_known_policy_plan"

dev_converged_runtime_policy_plan="$tmp_dir/dev-converged-runtime-policy.json"
jq '
  del(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda")) |
  (.resource_changes[] | select(.address == "module.service.aws_iam_role_policy.lambda") | .change) |= (
    .after.policy as $policy |
    .actions = ["no-op"] |
    .after = {
      id: "portfolio-lambda-dev-execution:portfolio-lambda-dev-runtime",
      name: "portfolio-lambda-dev-runtime",
      name_prefix: "",
      policy: $policy,
      role: "portfolio-lambda-dev-execution"
    } |
    .before = .after |
    .after_unknown = {}
  )
' "$dev_known_policy_plan" > "$dev_converged_runtime_policy_plan"

dev_converged_release_plan="$tmp_dir/dev-converged-release.json"
jq '
  def noop_resource($address; $type; $name): {
    mode: "managed",
    address: $address,
    type: $type,
    name: $name,
    change: {
      actions: ["no-op"],
      before: {name: $name},
      after: {name: $name},
      after_unknown: {},
      before_sensitive: false,
      after_sensitive: {}
    }
  };
  (.resource_changes[] | select(.mode == "managed") | .change) |= (
    .before = .after |
    .actions = ["no-op"] |
    .after_unknown = {}
  ) |
  (.resource_changes[] | select(.address == "module.service.aws_lambda_function.app") | .change) |= (
    .after.version = "8" |
    .before = .after
  ) |
  .resource_changes += [
    {
      mode: "managed",
      address: "module.service.aws_lambda_alias.live",
      type: "aws_lambda_alias",
      name: "live",
      change: {
        actions: ["no-op"],
        before: {name: "live", function_name: "portfolio-lambda-dev", function_version: "8"},
        after: {name: "live", function_name: "portfolio-lambda-dev", function_version: "8"},
        after_unknown: {},
        before_sensitive: false,
        after_sensitive: {}
      }
    },
    noop_resource(
      "module.service.aws_apigatewayv2_integration.lambda";
      "aws_apigatewayv2_integration";
      "lambda"
    ),
    noop_resource(
      "module.service.aws_apigatewayv2_route.default";
      "aws_apigatewayv2_route";
      "default"
    ),
    noop_resource(
      "module.service.aws_apigatewayv2_stage.default";
      "aws_apigatewayv2_stage";
      "default"
    ),
    noop_resource(
      "module.service.aws_lambda_permission.api";
      "aws_lambda_permission";
      "api"
    ),
    noop_resource(
      "module.service.aws_acm_certificate.custom[0]";
      "aws_acm_certificate";
      "custom"
    ),
    noop_resource(
      "module.service.aws_acm_certificate_validation.custom[0]";
      "aws_acm_certificate_validation";
      "custom"
    ),
    noop_resource(
      "module.service.aws_apigatewayv2_domain_name.custom[\"dev.craigdevjohnson.com\"]";
      "aws_apigatewayv2_domain_name";
      "custom"
    ),
    noop_resource(
      "module.service.aws_apigatewayv2_api_mapping.custom[\"dev.craigdevjohnson.com\"]";
      "aws_apigatewayv2_api_mapping";
      "custom"
    )
  ]
' "$dev_converged_runtime_policy_plan" > "$dev_converged_release_plan"

dev_rollback_plan="$tmp_dir/dev-rollback.json"
jq '
  .variables.live_version_override.value = 7 |
  (.resource_changes[] | select(.address == "module.service.aws_lambda_alias.live") | .change) |= (
    .actions = ["update"] |
    .before.function_version = "8" |
    .after.function_version = "7"
  )
' "$dev_converged_release_plan" > "$dev_rollback_plan"

dev_rollback_string_override_plan="$tmp_dir/dev-rollback-string-override.json"
jq '.variables.live_version_override.value = "7"' \
  "$dev_rollback_plan" > "$dev_rollback_string_override_plan"

dev_rollback_with_image_update_plan="$tmp_dir/dev-rollback-with-image-update.json"
jq --arg previous_image "$previous_release_image" '
  (.resource_changes[] | select(.address == "module.service.aws_lambda_function.app") | .change) |= (
    .actions = ["update"] |
    .before.image_uri = $previous_image
  )
' "$dev_rollback_plan" > "$dev_rollback_with_image_update_plan"

dev_rollback_with_log_update_plan="$tmp_dir/dev-rollback-with-log-update.json"
jq '
  (.resource_changes[] |
    select(.address == "module.service.aws_cloudwatch_log_group.lambda") |
    .change) |= (
      .actions = ["update"] |
      .before.retention_in_days = 7
    )
' "$dev_rollback_plan" > "$dev_rollback_with_log_update_plan"

dev_empty_policy_composition_plan="$tmp_dir/dev-empty-policy-composition.json"
jq '
  (.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change.after) |= (
    .source_json = "" |
    .override_json = "" |
    .source_policy_documents = [] |
    .override_policy_documents = []
  )
' "$dev_plan" > "$dev_empty_policy_composition_plan"

dev_deferred_runtime_policy_plan="$tmp_dir/dev-deferred-runtime-policy.json"
jq '
  (.resource_changes[] | select(
    .address == "module.service.aws_dynamodb_table.google_connections" or
    .address == "module.service.aws_dynamodb_table.soccer_sessions" or
    .address == "module.service.aws_cloudwatch_log_group.lambda"
  ) | .change) |= (
    .after.arn = null |
    .after_unknown.arn = true
  ) |
  (.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change) |= (
    .after.statement[0].resources = [null] |
    .after.statement[1].resources = [null] |
    .after.statement[4].resources = [null] |
    .after_unknown.statement = [{
      actions: [false, false, false],
      condition: [],
      not_principals: [],
      principals: [],
      resources: [true]
    }, {
      actions: [false],
      condition: [],
      not_principals: [],
      principals: [],
      resources: [true]
    }, {
      actions: [false],
      condition: [],
      not_principals: [],
      principals: [],
      resources: [false, false, false]
    }, {
      actions: [false],
      condition: [],
      not_principals: [],
      principals: [],
      resources: [false]
    }, {
      actions: [false, false],
      condition: [],
      not_principals: [],
      principals: [],
      resources: [true]
    }]
  ) |
  (.resource_changes[] | select(.address == "module.service.aws_iam_role_policy.lambda") | .change.after_unknown) = {
    id: true,
    name_prefix: true,
    policy: true,
    role: true
  }
' "$dev_plan" > "$dev_deferred_runtime_policy_plan"

dev_partial_state_runtime_policy_plan="$tmp_dir/dev-partial-state-runtime-policy.json"
jq '
  (.resource_changes[] | select(.address == "module.service.aws_cloudwatch_log_group.lambda") | .change) |= (
    .after.arn = null |
    .after_unknown.arn = true
  ) |
  (.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change) |= (
    .after.statement[4].resources = [null] |
    .after_unknown.statement = [
      {actions: [false, false, false], condition: [], not_principals: [], principals: [], resources: [false]},
      {actions: [false], condition: [], not_principals: [], principals: [], resources: [false]},
      {actions: [false], condition: [], not_principals: [], principals: [], resources: [false, false, false]},
      {actions: [false], condition: [], not_principals: [], principals: [], resources: [false]},
      {actions: [false, false], condition: [], not_principals: [], principals: [], resources: [true]}
    ]
  ) |
  (.resource_changes[] | select(.address == "module.service.aws_iam_role_policy.lambda") | .change) |= (
    .after.role = "portfolio-lambda-dev-execution" |
    .after_unknown = {id: true, name_prefix: true, policy: true}
  )
' "$dev_plan" > "$dev_partial_state_runtime_policy_plan"

dev_provider_empty_alarm_actions_plan="$tmp_dir/dev-provider-empty-alarm-actions.json"
jq '
  (.resource_changes[] | select(.type == "aws_cloudwatch_metric_alarm") | .change) |= (
    .after.alarm_actions = null |
    del(.after_unknown.alarm_actions)
  )
' "$dev_plan" > "$dev_provider_empty_alarm_actions_plan"

prod_provider_known_alarm_actions_plan="$tmp_dir/prod-provider-known-alarm-actions.json"
jq '
  (.resource_changes[] | select(.type == "aws_cloudwatch_metric_alarm") | .change) |= (
    .after_unknown.alarm_actions = [false]
  )
' "$prod_plan" > "$prod_provider_known_alarm_actions_plan"

dev_null_acm_private_key_plan="$tmp_dir/dev-null-acm-private-key.json"
jq '.resource_changes += [{
  mode: "managed",
  address: "module.service.aws_acm_certificate.custom[0]",
  type: "aws_acm_certificate",
  name: "custom",
  change: {
    actions: ["create"],
    before: null,
    after: {
      domain_name: "dev.craigdevjohnson.com",
      private_key: null
    },
    after_unknown: {},
    before_sensitive: false,
    after_sensitive: {private_key: true}
  }
}]' "$dev_plan" > "$dev_null_acm_private_key_plan"

expect_pass "artifact repository, lifecycle, and pull-policy plan" run_check "$artifact_plan" artifacts
expect_pass "exact GitHub Actions role plan" run_ci_roles_check "$ci_roles_plan"
expect_pass "converged GitHub Actions role no-op plan" run_ci_roles_check "$ci_roles_noop_plan"
expect_fail "GitHub Actions role no-op plan rejects a description" \
  run_ci_roles_check \
  "$ci_roles_description_noop_plan"
expect_fail "GitHub Actions role no-op plan rejects a name prefix" \
  run_ci_roles_check \
  "$ci_roles_name_prefix_noop_plan"
expect_fail "GitHub Actions role no-op plan rejects a permissions boundary" \
  run_ci_roles_check \
  "$ci_roles_boundary_noop_plan"
expect_fail "GitHub Actions role policy no-op plan rejects a name prefix" \
  run_ci_roles_check \
  "$ci_roles_policy_name_prefix_noop_plan"
expect_pass "refreshed GitHub Actions role no-op plan" \
  run_ci_roles_check \
  "$ci_roles_refreshed_noop_plan"
expect_pass "safe GitHub Actions role policy narrowing update" run_ci_roles_check "$ci_roles_narrow_update_plan"

ci_roles_data_read_plan="$tmp_dir/ci-roles-data-read.json"
jq '.resource_changes += [{
  mode: "data",
  address: "data.aws_iam_policy_document.release_trust",
  type: "aws_iam_policy_document",
  name: "release_trust",
  provider_name: "registry.opentofu.org/hashicorp/aws",
  change: {
    actions: ["read"],
    before: null,
    after: {},
    after_unknown: {},
    before_sensitive: false,
    after_sensitive: {}
  }
}]' "$ci_roles_plan" > "$ci_roles_data_read_plan"
expect_pass \
  "CI role plan accepts an approved IAM policy-document data read" \
  run_ci_roles_check \
  "$ci_roles_data_read_plan"

mutate_ci_roles_and_reject() {
  name=$1
  filter=$2
  mutated="$tmp_dir/ci-roles-mutated.json"
  jq "$filter" "$ci_roles_plan" > "$mutated"
  expect_fail "$name" run_ci_roles_check "$mutated"
}

mutate_ci_roles_and_reject "CI role plan rejects delete actions" '
  (.resource_changes[] | select(.address == "aws_iam_role.ci[\"release\"]") | .change.actions) = ["delete"]
'
mutate_ci_roles_and_reject "CI role plan rejects replacement actions" '
  (.resource_changes[] | select(.address == "aws_iam_role.ci[\"release\"]") | .change.actions) = ["delete", "create"]
'
mutate_ci_roles_and_reject "CI role plan rejects extra managed resources" '
  .resource_changes += [{
    mode: "managed",
    address: "aws_iam_user.unapproved",
    type: "aws_iam_user",
    name: "unapproved",
    provider_name: "registry.opentofu.org/hashicorp/aws",
    change: {
      actions: ["create"],
      before: null,
      after: {name: "unapproved"},
      after_unknown: {arn: true, id: true},
      before_sensitive: false,
      after_sensitive: {}
    }
  }]
'
mutate_ci_roles_and_reject "CI role plan rejects widened release ECR actions" '
  (.resource_changes[] | select(.address == "aws_iam_role_policy.release") | .change.after.policy) |= (
    fromjson |
    (.Statement[] |
      select(.Resource == "arn:aws:ecr:us-west-2:180294223248:repository/portfolio-lambda-releases") |
      .Action) += ["ecr:DeleteRepository"] |
    tojson
  )
'
mutate_ci_roles_and_reject "CI role plan rejects wildcard alarm reads" '
  (.resource_changes[] |
    select(.address == "aws_iam_role_policy.environment[\"dev\"]") |
    .change.after.policy) |= (
    fromjson |
    (.Statement[] | select(.Sid == "AlarmRead") | .Resource) =
      ["arn:aws:cloudwatch:us-west-2:180294223248:alarm:*"] |
    tojson
  )
'
mutate_ci_roles_and_reject "CI role plan rejects production mutation actions" '
  (.resource_changes[] | select(.address == "aws_iam_role_policy.environment[\"prod\"]") | .change.after.policy) |= (
    fromjson |
    (.Statement[] | select(.Sid == "LambdaRead") | .Action) += ["lambda:UpdateFunctionCode"] |
    tojson
  )
'
mutate_ci_roles_and_reject "CI role plan rejects wildcard GitHub trust subjects" '
  (.resource_changes[] | select(.address == "aws_iam_role.ci[\"release\"]") | .change.after.assume_role_policy) |= (
    fromjson |
    .Statement[0].Condition.StringEquals["token.actions.githubusercontent.com:sub"] =
      "repo:CraigDevJohnson/portfolio:*" |
    tojson
  )
'
mutate_ci_roles_and_reject "CI role plan rejects unknown inline policies" '
  (.resource_changes[] | select(.address == "aws_iam_role_policy.environment[\"prod\"]") | .change) |= (
    .after.policy = null |
    .after_unknown.policy = true
  )
'
mutate_ci_roles_and_reject "CI role plan rejects unknown trust policies" '
  (.resource_changes[] | select(.address == "aws_iam_role.ci[\"prod\"]") | .change) |= (
    .after.assume_role_policy = null |
    .after_unknown.assume_role_policy = true
  )
'
mutate_ci_roles_and_reject "CI role plan rejects conditional unknown policy targets" '
  (.resource_changes[] | select(.address == "aws_iam_role_policy.release") | .change) |= (
    .after.role = null |
    .after_unknown.role = true
  )
'
mutate_ci_roles_and_reject "CI role plan rejects configured managed policy attachments hidden by create unknowns" '
  (.configuration.root_module.resources[] |
    select(.address == "aws_iam_role.ci") |
    .expressions.managed_policy_arns) = {
    references: ["each.key"]
  }
'
mutate_ci_roles_and_reject "CI role plan rejects provisioners on approved resources" '
  (.configuration.root_module.resources[] | select(.address == "aws_iam_role.ci") | .provisioners) = [{
    type: "local-exec",
    expressions: {command: {constant_value: "echo unapproved"}}
  }]
'
mutate_ci_roles_and_reject "CI role plan rejects aliased or expanded AWS provider configuration" '
  .configuration.provider_config["aws.unapproved"] = {
    name: "aws",
    full_name: "registry.opentofu.org/hashicorp/aws",
    alias: "unapproved",
    version_constraint: "6.38.0",
    expressions: {
      allowed_account_ids: {constant_value: ["180294223248"]},
      profile: {constant_value: "portfolio-ci-roles-administrator"},
      region: {references: ["local.region"]}
    }
  } |
  (.configuration.root_module.resources[] |
    select(.address == "aws_iam_role.ci") |
    .provider_config_key) = "aws.unapproved"
'
mutate_ci_roles_and_reject "CI role plan rejects a changed AWS profile" '
  .configuration.provider_config.aws.expressions.profile.constant_value = "unapproved-profile"
'
mutate_ci_roles_and_reject "CI role plan rejects a changed allowed AWS account" '
  .configuration.provider_config.aws.expressions.allowed_account_ids.constant_value = ["999999999999"]
'
mutate_ci_roles_and_reject "CI role plan rejects a computed allowed AWS account list" '
  .configuration.provider_config.aws.expressions.allowed_account_ids = {references: ["local.account_id"]}
'
mutate_ci_roles_and_reject "CI role plan rejects sensitive file output expressions" '
  .configuration.root_module.outputs.local_secret = {
    sensitive: true,
    expression: {references: []}
  } |
  .output_changes.local_secret = {
    actions: ["create"],
    before: null,
    after: "redacted-test-value",
    after_unknown: false,
    before_sensitive: false,
    after_sensitive: true
  }
'
mutate_ci_roles_and_reject "CI role plan rejects stale deleted sensitive outputs" '
  .output_changes.stale_secret = {
    actions: ["delete"],
    before: "redacted-test-value",
    after: null,
    after_unknown: false,
    before_sensitive: true,
    after_sensitive: false
  }
'

expect_pass "development replacement plan" run_check "$dev_plan" dev
expect_pass "production replacement plan" run_check "$prod_plan" prod
expect_fail \
  "production replacement plan rejects an incomplete managed topology" \
  run_check "$prod_missing_integration_plan" prod
expect_fail \
  "production replacement plan rejects dormant unapproved configuration" \
  run_check "$prod_dormant_configuration_plan" prod
expect_fail \
  "production replacement plan rejects dormant root configuration" \
  run_check "$prod_dormant_root_configuration_plan" prod
expect_fail \
  "production replacement plan rejects a dormant sibling module" \
  run_check "$prod_dormant_sibling_module_plan" prod
expect_fail \
  "production replacement plan rejects a dormant nested module" \
  run_check "$prod_dormant_nested_module_plan" prod
expect_pass \
  "production plan with provider-known alarm action elements" \
  run_check "$prod_provider_known_alarm_actions_plan" prod
expect_pass "development plan with decoded runtime policy" run_check "$dev_known_policy_plan" dev
expect_pass "development plan with converged runtime policy" run_check "$dev_converged_runtime_policy_plan" dev
expect_pass "development plan with empty policy composition inputs" run_check "$dev_empty_policy_composition_plan" dev
expect_pass \
  "development plan with provider-deferred runtime resources" \
  run_check "$dev_deferred_runtime_policy_plan" dev
expect_pass \
  "development plan after partial role and table creation" \
  run_check "$dev_partial_state_runtime_policy_plan" dev
expect_pass \
  "development plan with provider-normalized empty alarm actions" \
  run_check "$dev_provider_empty_alarm_actions_plan" dev
expect_pass "ACM plan with null provider-sensitive private key" run_check "$dev_null_acm_private_key_plan" dev
expect_pass "automated release accepts only image and live-alias updates" run_maintenance_check "$dev_maintenance_plan"
expect_pass "automated release accepts an already-converged verified retry" \
  run_maintenance_check "$dev_converged_release_plan"
expect_pass "checked rollback accepts only the prior live alias version" \
  run_rollback_check "$dev_rollback_plan" 7
expect_pass "checked rollback accepts OpenTofu string-encoded numeric override" \
  run_rollback_check "$dev_rollback_string_override_plan" 7
expect_fail "checked rollback rejects a mismatched reviewed prior version" \
  run_rollback_check "$dev_rollback_plan" 6
expect_fail "checked rollback rejects an image update" \
  run_rollback_check "$dev_rollback_with_image_update_plan" 7
expect_fail "checked rollback rejects a second managed update" \
  run_rollback_check "$dev_rollback_with_log_update_plan" 7

converged_missing_resource_plan="$tmp_dir/converged-missing-resource.json"
jq 'del(.resource_changes[] | select(.address == "module.service.aws_lambda_permission.api"))' \
  "$dev_converged_release_plan" > "$converged_missing_resource_plan"
expect_fail \
  "automated converged release rejects a missing configured resource" \
  run_maintenance_check "$converged_missing_resource_plan"

converged_missing_certificate_plan="$tmp_dir/converged-missing-certificate.json"
jq 'del(.resource_changes[] | select(.address == "module.service.aws_acm_certificate.custom[0]"))' \
  "$dev_converged_release_plan" > "$converged_missing_certificate_plan"
expect_fail \
  "automated converged release rejects a missing configured certificate" \
  run_maintenance_check "$converged_missing_certificate_plan"

converged_missing_domain_mapping_plan="$tmp_dir/converged-missing-domain-mapping.json"
jq 'del(.resource_changes[] |
  select(.address == "module.service.aws_apigatewayv2_api_mapping.custom[\"dev.craigdevjohnson.com\"]"))' \
  "$dev_converged_release_plan" > "$converged_missing_domain_mapping_plan"
expect_fail \
  "automated converged release rejects a missing configured domain mapping" \
  run_maintenance_check "$converged_missing_domain_mapping_plan"

converged_duplicate_resource_plan="$tmp_dir/converged-duplicate-resource.json"
jq '.resource_changes += [
  first(.resource_changes[] | select(.address == "module.service.aws_lambda_permission.api"))
]' "$dev_converged_release_plan" > "$converged_duplicate_resource_plan"
expect_fail \
  "automated converged release rejects a duplicate configured resource" \
  run_maintenance_check "$converged_duplicate_resource_plan"

converged_wrong_digest_plan="$tmp_dir/converged-wrong-digest.json"
jq --arg image "$previous_release_image" '
  (.resource_changes[] |
    select(.address == "module.service.aws_lambda_function.app") |
    .change) |= (.before.image_uri = $image | .after.image_uri = $image)
' "$dev_converged_release_plan" > "$converged_wrong_digest_plan"
expect_fail \
  "automated converged release rejects a different image digest" \
  run_maintenance_check "$converged_wrong_digest_plan"

converged_alias_mismatch_plan="$tmp_dir/converged-alias-mismatch.json"
jq '
  (.resource_changes[] |
    select(.address == "module.service.aws_lambda_alias.live") |
    .change) |= (.before.function_version = "7" | .after.function_version = "7")
' "$dev_converged_release_plan" > "$converged_alias_mismatch_plan"
expect_fail \
  "automated converged release rejects an alias and version mismatch" \
  run_maintenance_check "$converged_alias_mismatch_plan"

converged_unknown_version_plan="$tmp_dir/converged-unknown-version.json"
jq '
  (.resource_changes[] |
    select(.address == "module.service.aws_lambda_function.app") |
    .change) |= (
      .before.version = null |
      .after.version = null |
      .after_unknown.version = true
    )
' "$dev_converged_release_plan" > "$converged_unknown_version_plan"
expect_fail \
  "automated converged release rejects an unknown published version" \
  run_maintenance_check "$converged_unknown_version_plan"

maintenance_provider_unknowns_plan="$tmp_dir/maintenance-provider-unknowns.json"
jq '
  (.resource_changes[] | select(.address == "module.service.aws_lambda_function.app") | .change) |= (
    .before.version = "7" |
    .after.version = null |
    .after_unknown.version = true
  )
' "$dev_maintenance_plan" > "$maintenance_provider_unknowns_plan"
expect_pass \
  "automated release accepts provider-computed version fields" \
  run_maintenance_check "$maintenance_provider_unknowns_plan"

maintenance_alias_override_plan="$tmp_dir/maintenance-alias-override.json"
jq '
  (.resource_changes[] | select(.address == "module.service.aws_lambda_alias.live") | .change) |= (
    .after.function_version = "999" |
    del(.after_unknown.function_version)
  )
' "$dev_maintenance_plan" > "$maintenance_alias_override_plan"
expect_fail \
  "automated release rejects an arbitrary known alias target" \
  run_maintenance_check "$maintenance_alias_override_plan"

maintenance_live_override_plan="$tmp_dir/maintenance-live-override.json"
jq '.variables.live_version_override.value = 999' "$dev_maintenance_plan" > "$maintenance_live_override_plan"
expect_fail "automated release rejects a live-version override" run_maintenance_check "$maintenance_live_override_plan"

maintenance_drift_plan="$tmp_dir/maintenance-drift.json"
jq '
  (.resource_changes[] |
    select(.address == "module.service.aws_lambda_function.app") |
    .change.after.timeout) = 30
' "$dev_maintenance_plan" > "$maintenance_drift_plan"
expect_fail "automated release rejects Lambda configuration drift" run_maintenance_check "$maintenance_drift_plan"

maintenance_unknown_drift_plan="$tmp_dir/maintenance-unknown-drift.json"
jq '
  (.resource_changes[] | select(.address == "module.service.aws_lambda_function.app") | .change) |= (
    .after.timeout = null |
    .after_unknown.timeout = true
  )
' "$dev_maintenance_plan" > "$maintenance_unknown_drift_plan"
expect_fail \
  "automated release rejects unapproved unknown configuration" \
  run_maintenance_check "$maintenance_unknown_drift_plan"

maintenance_unknown_tags_plan="$tmp_dir/maintenance-unknown-tags.json"
jq '(.resource_changes[] | select(.address == "module.service.aws_lambda_function.app") | .change) |= (
  .before.tags_all = {Environment: "dev", ManagedBy: "opentofu", Platform: "lambda-http-api", Project: "portfolio"} |
  .after.tags_all = null |
  .after_unknown.tags_all = true
)' "$dev_maintenance_plan" > "$maintenance_unknown_tags_plan"
expect_fail "automated release rejects unknown function tags" run_maintenance_check "$maintenance_unknown_tags_plan"

maintenance_create_plan="$tmp_dir/maintenance-create.json"
jq '
  (.resource_changes[] |
    select(.address == "module.service.aws_cloudwatch_log_group.lambda") |
    .change.actions) = ["create"]
' "$dev_maintenance_plan" > "$maintenance_create_plan"
expect_fail "automated release rejects standalone resource creation" run_maintenance_check "$maintenance_create_plan"

mutate_and_reject() {
  name=$1
  source=$2
  filter=$3
  mutated="$tmp_dir/mutated.json"
  jq "$filter" "$source" > "$mutated"
  environment=dev
  case "$source" in
    *artifact*) environment=artifacts ;;
    *prod*) environment=prod ;;
  esac
  expect_fail "$name" run_check "$mutated" "$environment"
}

mutate_and_reject "delete action" "$dev_plan" '.resource_changes[0].change.actions = ["delete"]'
mutate_and_reject "replace action" "$dev_plan" '.resource_changes[0].change.actions = ["delete", "create"]'
mutate_and_reject \
  "production plan rejects a forget action" \
  "$prod_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.aws_apigatewayv2_integration.lambda") |
    .change.actions) = ["forget"]'
mutate_and_reject \
  "production plan rejects a moved resource" \
  "$prod_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.aws_apigatewayv2_integration.lambda") |
    .previous_address) = "module.service.aws_apigatewayv2_integration.previous"'
mutate_and_reject \
  "production plan rejects a deposed resource" \
  "$prod_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.aws_apigatewayv2_integration.lambda") |
    .deposed) = "00000001"'
mutate_and_reject \
  "production plan rejects an importing resource" \
  "$prod_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.aws_apigatewayv2_integration.lambda") |
    .change.importing) = {id: "unreviewed"}'
mutate_and_reject \
  "production plan rejects generated configuration" \
  "$prod_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.aws_apigatewayv2_integration.lambda") |
    .change.generated_config) = "resource \"unreviewed\" \"example\" {}"'
mutate_and_reject \
  "legacy App Runner address" \
  "$dev_plan" \
  '.resource_changes[0].address = "module.service.aws_apprunner_service.legacy"'
mutate_and_reject "legacy resource name" "$dev_plan" '.resource_changes[2].change.after.function_name = "portfolio"'
mutate_and_reject \
  "mutable image tag" \
  "$dev_plan" \
  '.resource_changes[2].change.after.image_uri =
    "180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases:latest"'
mutate_and_reject \
  "development reserved concurrency drift" \
  "$dev_plan" \
  '.resource_changes[2].change.after.reserved_concurrent_executions = 5'
mutate_and_reject \
  "production reserved concurrency drift" \
  "$prod_plan" \
  '.resource_changes[2].change.after.reserved_concurrent_executions = -1'
mutate_and_reject "secret plan value" "$dev_plan" '.resource_changes[2].change.after.oauth_token = "do-not-store-this"'
mutate_and_reject \
  "secret prior-state value" \
  "$dev_plan" \
  '.resource_changes[2].change.before = {oauth_token: "do-not-store-this"}'
mutate_and_reject \
  "ACM plan with non-null sensitive private key" \
  "$dev_null_acm_private_key_plan" \
  '(.resource_changes[] |
    select(.type == "aws_acm_certificate") |
    .change.after.private_key) = "do-not-store-this"'
mutate_and_reject \
  "ACM plan with unmarked non-null private key" \
  "$dev_null_acm_private_key_plan" \
  '(.resource_changes[] |
    select(.type == "aws_acm_certificate") |
    .change) |= (
      .after.private_key = "do-not-store-this" |
      del(.after_sensitive.private_key)
    )'
mutate_and_reject \
  "ACM plan with missing sensitive private key" \
  "$dev_null_acm_private_key_plan" \
  '(.resource_changes[] |
    select(.type == "aws_acm_certificate") |
    .change) |= del(.after.private_key)'
mutate_and_reject \
  "ACM plan with deferred sensitive private key" \
  "$dev_null_acm_private_key_plan" \
  '(.resource_changes[] |
    select(.type == "aws_acm_certificate") |
    .change.after_unknown.private_key) = true'
mutate_and_reject \
  "ACM plan with unexpected null sensitive field" \
  "$dev_null_acm_private_key_plan" \
  '(.resource_changes[] |
    select(.type == "aws_acm_certificate") |
    .change) |= (
      .after.unreviewed = null |
      .after_sensitive.unreviewed = true
    )'
mutate_and_reject "root sensitive marker" "$dev_plan" '.resource_changes[0].change.after_sensitive = true'
mutate_and_reject \
  "missing execution boundary" \
  "$dev_plan" \
  '.resource_changes[0].change.after.permissions_boundary = null'
mutate_and_reject "wrong deterministic API name" "$dev_plan" '.resource_changes[3].change.after.name = "portfolio-http"'
mutate_and_reject \
  "development table protection drift" \
  "$dev_plan" \
  '.resource_changes[6].change.after.deletion_protection_enabled = true'
mutate_and_reject \
  "production table protection drift" \
  "$prod_plan" \
  '.resource_changes[6].change.after.deletion_protection_enabled = false'
mutate_and_reject "log retention drift" "$dev_plan" '.resource_changes[4].change.after.retention_in_days = 7'
mutate_and_reject "alarm action mismatch" "$prod_plan" '.resource_changes[8].change.after.alarm_actions = []'
mutate_and_reject \
  "production alarm actions reject a deferred provider element" \
  "$prod_provider_known_alarm_actions_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.aws_cloudwatch_metric_alarm.lambda_errors") |
    .change.after_unknown.alarm_actions[0]) = true'
mutate_and_reject \
  "production alarm actions reject a wrong-length provider marker" \
  "$prod_provider_known_alarm_actions_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.aws_cloudwatch_metric_alarm.lambda_errors") |
    .change.after_unknown.alarm_actions) += [false]'
mutate_and_reject \
  "production alarm actions reject a malformed provider marker" \
  "$prod_provider_known_alarm_actions_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.aws_cloudwatch_metric_alarm.lambda_errors") |
    .change.after_unknown.alarm_actions) = {"0": false}'
mutate_and_reject \
  "unknown empty alarm actions" \
  "$dev_provider_empty_alarm_actions_plan" \
  '.resource_changes[8].change.after_unknown.alarm_actions = true'
mutate_and_reject \
  "malformed empty alarm actions" \
  "$dev_provider_empty_alarm_actions_plan" \
  '.resource_changes[8].change.after.alarm_actions = false'
mutate_and_reject \
  "missing empty alarm actions" \
  "$dev_provider_empty_alarm_actions_plan" \
  'del(.resource_changes[8].change.after.alarm_actions)'
mutate_and_reject \
  "drifted root alarm action value" \
  "$dev_provider_empty_alarm_actions_plan" \
  '.variables.alarm_action_arns.value = null'
mutate_and_reject \
  "altered root alarm action reference" \
  "$dev_provider_empty_alarm_actions_plan" \
  '.configuration.root_module.module_calls.service.expressions.alarm_action_arns.references =
    ["var.unreviewed_alarm_actions"]'
mutate_and_reject \
  "altered resource alarm action reference" \
  "$dev_provider_empty_alarm_actions_plan" \
  '(.configuration.root_module.module_calls.service.module.resources[] |
    select(.address == "aws_cloudwatch_metric_alarm.lambda_errors") |
    .expressions.alarm_actions.references) = ["var.unreviewed_alarm_actions"]'
mutate_and_reject "alarm threshold drift" "$dev_plan" '.resource_changes[10].change.after.threshold = 29000'
mutate_and_reject "extra artifact resource" "$artifact_plan" '
  .resource_changes += [{
    address: "aws_iam_role.legacy",
    type: "aws_iam_role",
    name: "legacy",
    change: {actions: ["create"], after: {name: "legacy"}, after_sensitive: {}}
  }]'
mutate_and_reject \
  "artifact lifecycle drift" \
  "$artifact_plan" \
  '.resource_changes[1].change.after.policy = ({rules: []} | tojson)'
mutate_and_reject "artifact repository-policy drift" "$artifact_plan" '
  .resource_changes[2].change.after.policy = (
    {Version: "2012-10-17", Statement: []} |
    tojson
  )'
mutate_and_reject "unapproved IAM user resource" "$dev_plan" '
  .resource_changes += [{
    mode: "managed",
    address: "module.service.aws_iam_user.unapproved",
    type: "aws_iam_user",
    name: "unapproved",
    change: {actions: ["create"], after: {name: "unapproved"}, after_sensitive: {}}
  }]'
mutate_and_reject "unapproved SSM parameter resource" "$dev_plan" '
  .resource_changes += [{
    mode: "managed",
    address: "module.service.aws_ssm_parameter.unapproved",
    type: "aws_ssm_parameter",
    name: "unapproved",
    change: {
      actions: ["create"],
      after: {name: "/portfolio/lambda/dev/unapproved"},
      after_sensitive: {}
    }
  }]'

mutate_and_reject "development plan cannot use production runtime paths" "$dev_plan" '
  (.resource_changes[] |
    select(.address == "module.service.aws_lambda_function.app") |
    .change.after.environment[0].variables) |= with_entries(
    if (
      .key == "CLIENT_ID_KEY" or
      .key == "CLIENT_SECRET_KEY" or
      .key == "LPS_SESSION_KEY"
    ) then
      .value |= sub("/dev/"; "/prod/")
    else
      .
    end
  ) |
  (.resource_changes[] |
    select(.address == "module.service.data.aws_iam_policy_document.lambda") |
    .change.after.statement[] |
    select(.actions == ["ssm:GetParameters"]) |
    .resources[]) |= sub("/dev/"; "/prod/")'
mutate_and_reject "runtime policy rejects wildcard SSM resources" "$dev_plan" '
  (.resource_changes[] |
    select(.address == "module.service.data.aws_iam_policy_document.lambda") |
    .change.after.statement[] |
    select(.actions == ["ssm:GetParameters"]) |
    .resources) = ["arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/dev/*"]'
mutate_and_reject "runtime policy rejects altered actions" "$dev_plan" '
  (.resource_changes[] |
    select(.address == "module.service.data.aws_iam_policy_document.lambda") |
    .change.after.statement[] |
    select(.actions == ["ssm:GetParameters"]) |
    .actions) = ["ssm:GetParameter", "ssm:GetParameters"]'
mutate_and_reject "runtime policy rejects an unbound deferred value" "$dev_plan" '
  (.configuration.root_module.module_calls.service.module.resources[] |
    select(.address == "aws_iam_role_policy.lambda") |
    .expressions.policy.references) = ["var.unreviewed_policy"]'
mutate_and_reject "Lambda environment rejects an extra variable" "$dev_plan" '
  (.resource_changes[] |
    select(.address == "module.service.aws_lambda_function.app") |
    .change.after.environment[0].variables.UNREVIEWED) = "value"'
mutate_and_reject "decoded runtime policy rejects altered actions" "$dev_known_policy_plan" '
  (.resource_changes[] |
    select(.address == "module.service.aws_iam_role_policy.lambda") |
    .change.after.policy) |= (
      fromjson |
      (.Statement[] |
        select(.Action == "ssm:GetParameters") |
        .Action) = ["ssm:GetParameter", "ssm:GetParameters"] |
      tojson
    )'
mutate_and_reject \
  "converged runtime policy rejects altered actions" \
  "$dev_converged_runtime_policy_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.aws_iam_role_policy.lambda") |
    .change) |= (
      (.after.policy |= (
        fromjson |
        (.Statement[] |
          select(.Action == "ssm:GetParameters") |
          .Action) = ["ssm:GetParameter", "ssm:GetParameters"] |
        tojson
      )) |
      .before.policy = .after.policy
    )'
mutate_and_reject \
  "converged runtime policy rejects mismatched prior state" \
  "$dev_converged_runtime_policy_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.aws_iam_role_policy.lambda") |
    .change.before.role) = "unreviewed-role"'
mutate_and_reject \
  "converged runtime policy rejects partial unknown fields without its data read" \
  "$dev_converged_runtime_policy_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.aws_iam_role_policy.lambda") |
    .change) |= (
      .before.role = "unreviewed-role" |
      .after = {name: .after.name, policy: .after.policy} |
      .after_unknown = {id: true, name_prefix: true, role: true}
    )'
mutate_and_reject "runtime policy rejects injected source policy documents" "$dev_plan" '
  def injected_policy:
    {Version: "2012-10-17", Statement: [{Effect: "Allow", Action: "kms:Decrypt", Resource: "*"}]} |
    tojson;
  (.resource_changes[] |
    select(.address == "module.service.data.aws_iam_policy_document.lambda") |
    .change.after.source_policy_documents) = [injected_policy] |
  (.configuration.root_module.module_calls.service.module.resources[] |
    select(.address == "data.aws_iam_policy_document.lambda") |
    .expressions.source_policy_documents) = {constant_value: [injected_policy]}'
mutate_and_reject "runtime policy rejects injected source JSON" "$dev_plan" '
  (.resource_changes[] |
    select(.address == "module.service.data.aws_iam_policy_document.lambda") |
    .change.after.source_json) = ({Version: "2012-10-17", Statement: []} | tojson) |
  (.configuration.root_module.module_calls.service.module.resources[] |
    select(.address == "data.aws_iam_policy_document.lambda") |
    .expressions.source_json) = {constant_value: "injected"}'
mutate_and_reject "runtime policy rejects injected override JSON" "$dev_plan" '
  (.resource_changes[] |
    select(.address == "module.service.data.aws_iam_policy_document.lambda") |
    .change.after.override_json) = ({Version: "2012-10-17", Statement: []} | tojson)'
mutate_and_reject "runtime policy rejects injected override policy documents" "$dev_plan" '
  (.resource_changes[] |
    select(.address == "module.service.data.aws_iam_policy_document.lambda") |
    .change.after.override_policy_documents) = [({Version: "2012-10-17", Statement: []} | tojson)]'
mutate_and_reject "runtime policy rejects unknown source or override composition" "$dev_plan" '
  (.resource_changes[] |
    select(.address == "module.service.data.aws_iam_policy_document.lambda") |
    .change.after_unknown) += {
      source_json: true,
      override_json: true,
      source_policy_documents: true,
      override_policy_documents: true
    }'
mutate_and_reject "runtime policy rejects configured override composition" "$dev_plan" '
  (.configuration.root_module.module_calls.service.module.resources[] |
    select(.address == "data.aws_iam_policy_document.lambda") |
    .expressions.override_policy_documents) = {references: ["var.unreviewed_policy"]}'
mutate_and_reject "runtime policy rejects duplicate KMS alias configuration" "$dev_plan" '
  .configuration.root_module.module_calls.service.module.resources += [{
    address: "data.aws_kms_alias.ssm",
    mode: "data",
    type: "aws_kms_alias",
    name: "ssm",
    expressions: {name: {constant_value: "alias/customer-managed"}}
  }]'
mutate_and_reject "runtime policy rejects changed KMS alias and resource" "$dev_plan" '
  (.resource_changes[] |
    select(.address == "module.service.data.aws_iam_policy_document.lambda") |
    .change.after.statement[] |
    select(.actions == ["kms:Decrypt"]) |
    .resources) = [
      "arn:aws:kms:us-west-2:180294223248:key/11111111-1111-1111-1111-111111111111"
    ] |
  (.configuration.root_module.module_calls.service.module.resources[] |
    select(.address == "data.aws_kms_alias.ssm") |
    .expressions.name.constant_value) = "alias/customer-managed"'
mutate_and_reject "runtime policy rejects changed structured KMS reference" "$dev_plan" '
  (.configuration.root_module.module_calls.service.module.resources[] |
    select(.address == "data.aws_iam_policy_document.lambda") |
    .expressions.statement[] |
    select(.actions.constant_value == ["kms:Decrypt"]) |
    .resources.references) = ["var.unreviewed_kms_key"]'
mutate_and_reject "runtime policy rejects changed structured action" "$dev_plan" '
  (.configuration.root_module.module_calls.service.module.resources[] |
    select(.address == "data.aws_iam_policy_document.lambda") |
    .expressions.statement[] |
    select(.actions.constant_value == ["ssm:GetParameters"]) |
    .actions.constant_value) = ["ssm:GetParameter", "ssm:GetParameters"]'
mutate_and_reject \
  "runtime policy rejects a deferred ARN without its provider unknown marker" \
  "$dev_deferred_runtime_policy_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.aws_dynamodb_table.google_connections") |
    .change.after_unknown.arn) = false'
mutate_and_reject \
  "runtime policy rejects a deferred policy resource without its paired unknown marker" \
  "$dev_deferred_runtime_policy_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.data.aws_iam_policy_document.lambda") |
    .change.after_unknown.statement[0].resources) = [false]'
mutate_and_reject \
  "runtime policy rejects a deferred statement effect" \
  "$dev_deferred_runtime_policy_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.data.aws_iam_policy_document.lambda") |
    .change.after_unknown.statement[0].effect) = true'
mutate_and_reject \
  "runtime policy rejects extra deferred inline-policy fields" \
  "$dev_deferred_runtime_policy_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.aws_iam_role_policy.lambda") |
    .change.after_unknown.unreviewed) = true'
mutate_and_reject \
  "runtime policy rejects an unbound deferred table ARN" \
  "$dev_deferred_runtime_policy_plan" \
  '(.configuration.root_module.module_calls.service.module.resources[] |
    select(.address == "data.aws_iam_policy_document.lambda") |
    .expressions.statement[] |
    select(.actions.constant_value == [
      "dynamodb:GetItem",
      "dynamodb:PutItem",
      "dynamodb:DeleteItem"
    ]) |
    .resources.references) = ["var.unreviewed_table_arn"]'
mutate_and_reject \
  "partial runtime policy rejects the wrong known role" \
  "$dev_partial_state_runtime_policy_plan" \
  '(.resource_changes[] |
    select(.address == "module.service.aws_iam_role_policy.lambda") |
    .change.after.role) = "unreviewed-role"'

dev_domain_plan="$tmp_dir/dev-domain.json"
jq '.resource_changes += [
  {
    mode: "managed",
    address: "module.service.aws_acm_certificate.custom[0]",
    type: "aws_acm_certificate",
    name: "custom",
    change: {
      actions: ["create"],
      after: {domain_name: "dev.craigdevjohnson.com"},
      after_sensitive: {}
    }
  },
  {
    mode: "managed",
    address: "module.service.aws_acm_certificate_validation.custom[0]",
    type: "aws_acm_certificate_validation",
    name: "custom",
    change: {actions: ["create"], after: {}, after_sensitive: {}}
  },
  {
    mode: "managed",
    address: "module.service.aws_apigatewayv2_domain_name.custom[\"dev.craigdevjohnson.com\"]",
    type: "aws_apigatewayv2_domain_name",
    name: "custom",
    change: {
      actions: ["create"],
      after: {domain_name: "dev.craigdevjohnson.com"},
      after_sensitive: {}
    }
  },
  {
    mode: "managed",
    address: "module.service.aws_apigatewayv2_api_mapping.custom[\"dev.craigdevjohnson.com\"]",
    type: "aws_apigatewayv2_api_mapping",
    name: "custom",
    change: {actions: ["create"], after: {}, after_sensitive: {}}
  }
]' "$dev_plan" > "$dev_domain_plan"
expect_pass "approved conditional development domain resources" run_check "$dev_domain_plan" dev
mutate_and_reject \
  "unapproved conditional domain address" \
  "$dev_domain_plan" \
  '.resource_changes[-1].address =
    "module.service.aws_apigatewayv2_api_mapping.custom[\"attacker.example\"]"'

expect_pass "exact reviewed IAM policy-document data read" run_check "$dev_plan" dev
mutate_and_reject "unapproved data-source address" "$dev_plan" '
  (.resource_changes[] |
    select(.mode == "data") |
    .address) = "module.service.data.aws_iam_policy_document.unapproved"'
mutate_and_reject "unapproved data-source type" "$dev_plan" '
  (.resource_changes[] |
    select(.mode == "data") |
    .type) = "aws_caller_identity"'
mutate_and_reject "data source presented as a managed resource" "$dev_plan" '
  (.resource_changes[] |
    select(.mode == "data") |
    .mode) = "managed"'
mutate_and_reject "data source with a non-read action" "$dev_plan" '
  (.resource_changes[] |
    select(.mode == "data") |
    .change.actions) = ["create"]'

documented_versioning_mutation_is_guarded() {
  document=$1
  perl -0ne '
    my $found = 0;
    while (/```bash\n(.*?)```/sg) {
      my $block = $1;
      next unless $block =~ /s3api put-bucket-versioning/;
      exit 1 unless $block =~ /task lambda-artifacts-init.*s3api put-bucket-versioning/s;
      $found = 1;
    }
    exit($found ? 0 : 1);
  ' "$document"
}

expect_pass \
  "deployment guide guards bucket versioning immediately before mutation" \
  documented_versioning_mutation_is_guarded \
  "$repo_root/DEPLOY-INSTRUCTIONS.md"
expect_pass \
  "authoritative plan guards bucket versioning immediately before mutation" \
  documented_versioning_mutation_is_guarded \
  "$repo_root/docs/superpowers/plans/2026-08-21-development-lambda-cutover.md"

documented_parameter_streams_are_fail_closed() {
  document=$1
  perl -0ne '
    my ($task) = /(### Task 10:.*?)(?=\n---\n\n### Task 11:)/s;
    exit 1 unless defined $task;
    exit 1 if $task =~ /--cli-input-json file:\/\/\/dev\/stdin/;

    my @blocks = grep { /ssm put-parameter/ } ($task =~ /```bash\n(.*?)```/sg);
    exit 1 unless @blocks == 2;

    for my $block (@blocks) {
      exit 1 unless $block =~ /^set -euo pipefail$/m;
      exit 1 unless $block =~ /^set \+x$/m;
      exit 1 unless $block =~ /aws --profile "\$AWS_PROFILE" configure get cli_history/;
      exit 1 unless $block =~ /\nassert_aws_cli_history_disabled\n.*(?:openssl rand|--with-decryption)/s;
      exit 1 unless (() = $block =~ /ssm put-parameter/g) == 1;
      exit 1 unless $block =~ /--no-overwrite/;
      exit 1 unless $block =~ /--value file:\/\/\/dev\/stdin/;
    }

    my ($session) = grep { /openssl rand/ } @blocks;
    my ($oauth) = grep { /--with-decryption/ } @blocks;
    exit 1 unless defined $session && defined $oauth;
    exit 1 unless $session =~ /--name \/portfolio\/lambda\/dev\/LPS_SESSION_KEY/;
    exit 1 unless $oauth =~ /for name in CLIENT_ID_KEY CLIENT_SECRET_KEY; do/;
    exit 1 unless $oauth =~ /--name "\/portfolio\/lambda\/dev\/\$name"/;
    exit 1 unless $oauth =~ /--name "\/portfolio\/\$name"/;
    exit 0;
  ' "$document"
}

expect_pass \
  "parameter streams are non-overwriting and fail closed on CLI history" \
  documented_parameter_streams_are_fail_closed \
  "$repo_root/docs/superpowers/plans/2026-08-21-development-lambda-cutover.md"

fake_bin="$tmp_dir/fake-bin"
mkdir "$fake_bin"
command_log="$tmp_dir/commands.log"
: > "$command_log"

cat > "$fake_bin/aws" << 'EOF'
#!/bin/sh
set -eu
fake_digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
printf 'aws %s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
  *"sts get-caller-identity"*"--query Arn"*)
    printf '%s\n' "${FAKE_ARN:-arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioDeployer_abc/craig}"
    ;;
  *"sts get-caller-identity"*"--query Account"*)
    printf '%s\n' "${FAKE_ACCOUNT:-180294223248}"
    ;;
  *"iam get-role"*"--query Role.Arn"*)
    role_name=
    while [ "$#" -gt 0 ]; do
      if [ "$1" = --role-name ]; then
        shift
        role_name=$1
      fi
      shift
    done
    test -n "$role_name"
    printf 'arn:aws:iam::%s:role/%s\n' \
      "${FAKE_ROLE_ACCOUNT:-180294223248}" "$role_name"
    ;;
  *"ecr get-login-password"*)
    printf 'fake-password\n'
    ;;
  *"ecr describe-repositories"*)
    printf '%s\n' "${FAKE_REPOSITORY_MUTABILITY:-IMMUTABLE}"
    ;;
  *"ecr wait image-scan-complete"*)
    case "${FAKE_WAITER_MODE:-complete}" in
      complete) ;;
      denied)
        printf 'An error occurred (AccessDeniedException) while waiting for image scan completion: denied\n' >&2
        exit 254
        ;;
      failed)
        printf 'Waiter ImageScanComplete failed: terminal scan status FAILED\n' >&2
        exit 255
        ;;
      *)
        printf 'invalid FAKE_WAITER_MODE\n' >&2
        exit 1
        ;;
    esac
    ;;
  *"ecr describe-image-scan-findings"*)
    case "${FAKE_SCAN_MODE:-complete}" in
      complete)
        printf '%s\n' '{"ScanStatus":"COMPLETE","FindingSeverityCounts":{"CRITICAL":0,"HIGH":0}}'
        ;;
      missing-once)
        if [ ! -f "$FAKE_SCAN_LOOKUP_STATE" ]; then
          : >"$FAKE_SCAN_LOOKUP_STATE"
          printf 'An error occurred (ScanNotFoundException) when calling the ' >&2
          printf 'DescribeImageScanFindings operation: scan does not exist yet\n' >&2
          exit 254
        fi
        printf '%s\n' '{"ScanStatus":"COMPLETE","FindingSeverityCounts":{}}'
        ;;
      missing)
        printf 'An error occurred (ScanNotFoundException) when calling the ' >&2
        printf 'DescribeImageScanFindings operation: scan does not exist yet\n' >&2
        exit 254
        ;;
      ambiguous)
        printf 'AccessDeniedException included the words ScanNotFoundException\n' >&2
        exit 254
        ;;
      denied)
        printf 'An error occurred (AccessDeniedException) when calling the ' >&2
        printf 'DescribeImageScanFindings operation: denied\n' >&2
        exit 254
        ;;
      failed)
        printf '%s\n' '{"ScanStatus":"FAILED","FindingSeverityCounts":{}}'
        ;;
      *)
        printf 'invalid FAKE_SCAN_MODE\n' >&2
        exit 1
        ;;
    esac
    ;;
  *"ecr describe-images"*"{Digest:imageDigest"*)
    printf '{"Digest":"%s","PushedAt":"2026-08-22T00:00:00Z","ScanStatus":"COMPLETE"}\n' \
      "$fake_digest"
    ;;
  *"ecr describe-images"*"imageDigest"*)
    if [ ! -f "$FAKE_LOOKUP_STATE" ]; then
      : >"$FAKE_LOOKUP_STATE"
      case "${FAKE_LOOKUP_MODE:-absent}" in
        absent)
          printf 'An error occurred (ImageNotFoundException) when calling the ' >&2
          printf 'DescribeImages operation: image does not exist\n' >&2
          exit 254
          ;;
        denied)
          printf 'An error occurred (AccessDeniedException) when calling the DescribeImages operation: denied\n' >&2
          exit 254
          ;;
        ambiguous)
          printf 'AccessDeniedException included the words ImageNotFoundException\n' >&2
          exit 254
          ;;
        existing) printf '%s\n' "${FAKE_EXISTING_DIGEST:-$fake_digest}" ;;
        *) printf 'invalid FAKE_LOOKUP_MODE\n' >&2; exit 1 ;;
      esac
    else
      printf '%s\n' "${FAKE_PUSHED_DIGEST:-$fake_digest}"
    fi
    ;;
  *"ecr describe-images"*)
    printf '{"Digest":"%s","PushedAt":"2026-08-22T00:00:00Z","ScanStatus":"COMPLETE"}\n' \
      "$fake_digest"
    ;;
  *)
    printf 'unexpected fake aws command: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

cat > "$fake_bin/tofu" << 'EOF'
#!/bin/sh
set -eu
printf 'tofu %s\n' "$*" >>"$COMMAND_LOG"
if [ -n "${TF_CLI_CONFIG_FILE:-}" ]; then
  test -f "$TF_CLI_CONFIG_FILE"
  test ! -s "$TF_CLI_CONFIG_FILE"
  printf 'tofu-cli-config %s\n' "$TF_CLI_CONFIG_FILE" >> "$COMMAND_LOG"
fi
case "$*" in
  *" init "*)
    if [ -n "${TF_DATA_DIR:-}" ]; then
      mkdir -p "$TF_DATA_DIR"
      backend_extra=${FAKE_BACKEND_EXTRA_JSON:-}
      [ -n "$backend_extra" ] || backend_extra='{}'
      jq -nc \
        --argjson extra "$backend_extra" '
          {
            backend: {
              type: "s3",
              config: ({
                bucket: "portfolio-tofu-state-180294223248",
                key: "portfolio-lambda-http-api/ci-roles/terraform.tfstate",
                region: "us-west-2",
                encrypt: true,
                use_lockfile: true
              } + $extra)
            }
          }
        ' > "$TF_DATA_DIR/terraform.tfstate"
    fi
    ;;
  *" workspace show"*) printf '%s\n' "${FAKE_WORKSPACE:-default}" ;;
  *" plan "*)
    for argument in "$@"; do
      case "$argument" in
        -out=*) : >"${argument#-out=}" ;;
      esac
    done
    if [ -n "${FAKE_PLAN_OUTPUT:-}" ]; then
      printf '%s\n' "$FAKE_PLAN_OUTPUT"
    fi
    ;;
  *" show -json "*)
    if [ "${FAKE_TOFU_SIGNAL_PARENT:-false}" = true ]; then
      kill -TERM "$PPID"
      exit 143
    fi
    cat "$FAKE_PLAN_JSON"
    ;;
  *" show -no-color "*) printf 'synthetic human-readable plan\n' ;;
  *" apply "*)
    if [ "${FAKE_SIGNAL_APPLY_PARENT:-false}" = true ]; then
      kill -TERM "$PPID"
    fi
    ;;
  *" output -raw ecr_repository_url"*)
    printf '%s\n' '180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases'
    ;;
  *)
    printf 'unexpected fake tofu command: %s\n' "$*" >&2
    exit 1
    ;;
esac
EOF

cat > "$fake_bin/git" << 'EOF'
#!/bin/sh
set -eu
printf 'git %s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
  "status --porcelain") ;;
  "rev-parse HEAD") printf '0123456789abcdef0123456789abcdef01234567\n' ;;
  *) /usr/bin/git "$@" ;;
esac
EOF

cat > "$fake_bin/docker" << 'EOF'
#!/bin/sh
set -eu
printf 'docker %s\n' "$*" >>"$COMMAND_LOG"
case "$1" in
  login) cat >/dev/null ;;
  tag) ;;
  push) [ "${FAKE_PUSH_FAIL:-false}" != true ] ;;
  *) printf 'unexpected fake docker command: %s\n' "$*" >&2; exit 1 ;;
esac
EOF

cat > "$fake_bin/task" << 'EOF'
#!/bin/sh
set -eu
printf 'task %s\n' "$*" >>"$COMMAND_LOG"
test "$1" = build-lambda-image
EOF
cat > "$fake_bin/sleep" << 'EOF'
#!/bin/sh
set -eu
printf 'sleep %s\n' "$*" >>"$COMMAND_LOG"
EOF
cat > "$fake_bin/shasum" << 'EOF'
#!/bin/sh
set -eu
if [ -n "${FAKE_MUTATE_PLAN_PATH:-}" ]; then
  for argument in "$@"; do
    if [ "$argument" = "$FAKE_MUTATE_PLAN_PATH" ]; then
      printf 'replacement plan bytes\n' > "$FAKE_MUTATE_PLAN_PATH"
    fi
  done
fi
exec "${REAL_SHASUM:-/usr/bin/shasum}" "$@"
EOF
chmod +x "$fake_bin"/*

real_task=$(command -v task)
real_shasum=$(command -v shasum)
lookup_state="$tmp_dir/ecr-lookup-state"
scan_lookup_state="$tmp_dir/ecr-scan-lookup-state"
run_task() {
  rm -f "$lookup_state"
  rm -f "$scan_lookup_state"
  env -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
    PATH="$fake_bin:$PATH" \
    COMMAND_LOG="$command_log" \
    FAKE_PLAN_JSON="${TASK7_PLAN_JSON:-$dev_plan}" \
    FAKE_EXISTING_DIGEST="${FAKE_EXISTING_DIGEST:-$release_digest}" \
    FAKE_LOOKUP_MODE="${FAKE_LOOKUP_MODE:-absent}" \
    FAKE_LOOKUP_STATE="$lookup_state" \
    FAKE_PUSHED_DIGEST="${FAKE_PUSHED_DIGEST:-$release_digest}" \
    FAKE_PUSH_FAIL="${FAKE_PUSH_FAIL:-false}" \
    FAKE_REPOSITORY_MUTABILITY="${FAKE_REPOSITORY_MUTABILITY:-IMMUTABLE}" \
    FAKE_SCAN_MODE="${FAKE_SCAN_MODE:-complete}" \
    FAKE_SCAN_LOOKUP_STATE="$scan_lookup_state" \
    FAKE_WAITER_MODE="${FAKE_WAITER_MODE:-complete}" \
    AWS_PROFILE=portfolio-deployer \
    AWS_REGION=us-west-2 \
    "$real_task" --dir "$repo_root" "$@"
}

run_ci_roles_task() {
  ci_task=$1
  ci_profile=$2
  ci_region=$3
  ci_account=$4
  ci_arn=$5
  shift 5
  env -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
    PATH="$fake_bin:$PATH" \
    COMMAND_LOG="$command_log" \
    FAKE_PLAN_JSON="${TASK7_PLAN_JSON:-$ci_roles_plan}" \
    FAKE_BACKEND_EXTRA_JSON="${FAKE_BACKEND_EXTRA_JSON:-}" \
    FAKE_MUTATE_PLAN_PATH="${FAKE_MUTATE_PLAN_PATH:-}" \
    FAKE_SIGNAL_APPLY_PARENT="${FAKE_SIGNAL_APPLY_PARENT:-false}" \
    REAL_SHASUM="$real_shasum" \
    FAKE_ACCOUNT="$ci_account" \
    FAKE_ARN="$ci_arn" \
    AWS_PROFILE="$ci_profile" \
    AWS_REGION="$ci_region" \
    APPROVED_CI_ROLES_ADMIN="${CI_ROLES_ACK:-portfolio-lambda-http-api/ci-roles}" \
    "$real_task" --dir "$repo_root" "$ci_task" "$@"
}

run_ci_roles_as_admin() {
  ci_task=$1
  shift
  run_ci_roles_task \
    "$ci_task" \
    portfolio-ci-roles-administrator \
    us-west-2 \
    180294223248 \
    arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioCIRolesAdministrator_abc/craig \
    "$@"
}

run_ci_roles_with_wrong_acknowledgement() (
  CI_ROLES_ACK=wrong-root
  export CI_ROLES_ACK
  run_ci_roles_as_admin lambda-ci-roles-init
)

run_ci_roles_task_with_ambient_credential() {
  ci_task=$1
  credential_name=$2
  credential_value=$3
  shift 3
  env -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
    "$credential_name=$credential_value" \
    PATH="$fake_bin:$PATH" \
    COMMAND_LOG="$command_log" \
    FAKE_PLAN_JSON="${TASK7_PLAN_JSON:-$ci_roles_plan}" \
    FAKE_ACCOUNT=180294223248 \
    FAKE_ARN=arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioCIRolesAdministrator_abc/craig \
    AWS_PROFILE=portfolio-ci-roles-administrator \
    AWS_REGION=us-west-2 \
    APPROVED_CI_ROLES_ADMIN=portfolio-lambda-http-api/ci-roles \
    "$real_task" --dir "$repo_root" "$ci_task" "$@"
}

sensitive_release_plan="$tmp_dir/sensitive-release-plan.json"
jq '(.resource_changes[] |
  select(.address == "module.service.aws_lambda_function.app") |
  .change.after.oauth_token) = "do-not-retain-this"' \
  "$dev_maintenance_plan" > "$sensitive_release_plan"
rejected_release_evidence="$tmp_dir/rejected-release-evidence"
rejected_release_output="$tmp_dir/rejected-release-output"
rejected_plan_log_sentinel=do-not-log-raw-release-plan-output
: > "$command_log"
if PATH="$fake_bin:$PATH" \
  COMMAND_LOG="$command_log" \
  FAKE_PLAN_OUTPUT="$rejected_plan_log_sentinel" \
  FAKE_PLAN_JSON="$sensitive_release_plan" \
  RELEASE_ENVIRONMENT=development \
  IMAGE_DIGEST="$release_digest" \
  EVIDENCE_DIR="$rejected_release_evidence" \
  ECR_URL="$release_repository" \
  sh "$repo_root/scripts/create-ci-lambda-release-plan.sh" > "$rejected_release_output" 2>&1; then
  printf 'FAIL: sensitive release plan passed policy validation\n' >&2
  exit 1
fi
test -f "$rejected_release_evidence/policy.txt"
test -f "$rejected_release_evidence/PLAN_NOT_APPROVED"
test "$(cat "$rejected_release_evidence/policy.txt")" = \
  'Lambda release plan rejected by policy; detailed diagnostics are withheld.' || {
  printf 'FAIL: rejected release published detailed policy diagnostics\n' >&2
  exit 1
}
grep -Fq 'tofu -chdir=infra/lambda/environments/dev plan' "$command_log"
for raw_plan_artifact in dev.tfplan plan.json plan.txt plan.sha256; do
  test ! -e "$rejected_release_evidence/$raw_plan_artifact" || {
    printf 'FAIL: rejected release retained raw artifact %s\n' "$raw_plan_artifact" >&2
    exit 1
  }
done
if grep -Fq "$rejected_plan_log_sentinel" "$rejected_release_output" ||
  grep -R -Fq "$rejected_plan_log_sentinel" "$rejected_release_evidence"; then
  printf 'FAIL: rejected release exposed raw plan command output\n' >&2
  exit 1
fi
if grep -Fq 'do-not-retain-this' "$rejected_release_output" ||
  grep -R -Fq 'do-not-retain-this' "$rejected_release_evidence"; then
  printf 'FAIL: rejected release evidence retained a sensitive value\n' >&2
  exit 1
fi
pass "release withholds raw evidence for a policy-rejected sensitive plan"

accepted_release_evidence="$tmp_dir/accepted-release-evidence"
PATH="$fake_bin:$PATH" \
  COMMAND_LOG="$command_log" \
  FAKE_PLAN_JSON="$dev_maintenance_plan" \
  RELEASE_ENVIRONMENT=development \
  IMAGE_DIGEST="$release_digest" \
  EVIDENCE_DIR="$accepted_release_evidence" \
  ECR_URL="$release_repository" \
  sh "$repo_root/scripts/create-ci-lambda-release-plan.sh" > /dev/null
test -f "$accepted_release_evidence/policy.txt"
test ! -e "$accepted_release_evidence/PLAN_NOT_APPROVED"
for accepted_plan_artifact in dev.tfplan plan.json plan.txt plan.sha256; do
  test -f "$accepted_release_evidence/$accepted_plan_artifact" || {
    printf 'FAIL: accepted release omitted artifact %s\n' "$accepted_plan_artifact" >&2
    exit 1
  }
done
(cd "$accepted_release_evidence" && sha256sum -c plan.sha256 > /dev/null) || {
  printf 'FAIL: accepted release checksum does not verify its published plan\n' >&2
  exit 1
}
pass "release publishes raw evidence only after policy approval"

rejected_rollback_evidence="$tmp_dir/rejected-rollback-evidence"
if PATH="$fake_bin:$PATH" \
  COMMAND_LOG="$command_log" \
  FAKE_PLAN_JSON="$dev_rollback_with_image_update_plan" \
  IMAGE_DIGEST="$release_digest" \
  PRIOR_VERSION=7 \
  EVIDENCE_DIR="$rejected_rollback_evidence" \
  ECR_URL="$release_repository" \
  sh "$repo_root/scripts/create-ci-lambda-rollback-plan.sh" > /dev/null 2>&1; then
  printf 'FAIL: rollback plan with an image update passed policy validation\n' >&2
  exit 1
fi
test -f "$rejected_rollback_evidence/ROLLBACK_NOT_APPROVED"
for rejected_rollback_artifact in rollback.tfplan rollback.json rollback.txt rollback.sha256; do
  test ! -e "$rejected_rollback_evidence/$rejected_rollback_artifact" || {
    printf 'FAIL: rejected rollback retained raw artifact %s\n' "$rejected_rollback_artifact" >&2
    exit 1
  }
done
pass "rollback withholds raw evidence for a policy-rejected plan"

accepted_rollback_evidence="$tmp_dir/accepted-rollback-evidence"
PATH="$fake_bin:$PATH" \
  COMMAND_LOG="$command_log" \
  FAKE_PLAN_JSON="$dev_rollback_plan" \
  IMAGE_DIGEST="$release_digest" \
  PRIOR_VERSION=7 \
  EVIDENCE_DIR="$accepted_rollback_evidence" \
  ECR_URL="$release_repository" \
  sh "$repo_root/scripts/create-ci-lambda-rollback-plan.sh" > /dev/null
test ! -e "$accepted_rollback_evidence/ROLLBACK_NOT_APPROVED"
for accepted_rollback_artifact in \
  rollback.tfplan rollback.json rollback.txt rollback.sha256 rollback-policy.txt; do
  test -f "$accepted_rollback_evidence/$accepted_rollback_artifact" || {
    printf 'FAIL: accepted rollback omitted artifact %s\n' "$accepted_rollback_artifact" >&2
    exit 1
  }
done
(cd "$accepted_rollback_evidence" && sha256sum -c rollback.sha256 > /dev/null) || {
  printf 'FAIL: accepted rollback checksum does not verify its published plan\n' >&2
  exit 1
}
pass "rollback publishes evidence only after strict policy approval"

expect_ci_roles_rejection() {
  name=$1
  shift
  : > "$command_log"
  expect_fail "$name" "$@"
  if grep -q '^tofu ' "$command_log"; then
    printf 'FAIL: %s invoked OpenTofu after rejecting the identity\n' "$name" >&2
    exit 1
  fi
}

expect_ci_roles_acceptance() {
  name=$1
  expected_tofu_command=$2
  shift 2
  : > "$command_log"
  expect_pass "$name" "$@"
  expected_identity_prefix='aws --profile portfolio-ci-roles-administrator --region us-west-2'
  expected_account_check="$expected_identity_prefix sts get-caller-identity --query Account --output text"
  expected_arn_check="$expected_identity_prefix sts get-caller-identity --query Arn --output text"
  grep -Fq "$expected_account_check" "$command_log" || {
    printf 'FAIL: %s did not bind the account check to the reviewed profile and region\n' "$name" >&2
    exit 1
  }
  grep -Fq "$expected_arn_check" "$command_log" || {
    printf 'FAIL: %s did not bind the ARN check to the reviewed profile and region\n' "$name" >&2
    exit 1
  }
  grep -Fq "$expected_tofu_command" "$command_log" || {
    printf 'FAIL: %s did not run the expected CI-role OpenTofu command\n' "$name" >&2
    exit 1
  }
}

expect_ci_roles_rejection "CI roles guard rejects a deployer session through a profile alias" \
  run_ci_roles_task \
  lambda-ci-roles-init \
  portfolio-ci-roles-administrator \
  us-west-2 \
  180294223248 \
  arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioDeployer_abc/craig
expect_ci_roles_rejection "CI roles guard requires the reviewed administrator profile" \
  run_ci_roles_task \
  lambda-ci-roles-init \
  renamed-ci-roles-administrator \
  us-west-2 \
  180294223248 \
  arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioCIRolesAdministrator_abc/craig
expect_ci_roles_rejection "CI roles guard requires the reviewed region" \
  run_ci_roles_task \
  lambda-ci-roles-init \
  portfolio-ci-roles-administrator \
  us-east-1 \
  180294223248 \
  arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioCIRolesAdministrator_abc/craig
expect_ci_roles_rejection "CI roles guard rejects the wrong AWS account" \
  run_ci_roles_task \
  lambda-ci-roles-init \
  portfolio-ci-roles-administrator \
  us-west-2 \
  111122223333 \
  arn:aws:sts::111122223333:assumed-role/AWSReservedSSO_PortfolioCIRolesAdministrator_abc/craig
expect_ci_roles_rejection "CI roles guard rejects root" \
  run_ci_roles_task \
  lambda-ci-roles-init \
  portfolio-ci-roles-administrator \
  us-west-2 \
  180294223248 \
  arn:aws:iam::180294223248:root
expect_ci_roles_rejection "CI roles guard rejects the wrong root acknowledgement" \
  run_ci_roles_with_wrong_acknowledgement
expect_ci_roles_rejection "CI roles guard rejects ambient access-key credentials" \
  run_ci_roles_task_with_ambient_credential lambda-ci-roles-init AWS_ACCESS_KEY_ID AKIASTATIC
expect_ci_roles_rejection "CI roles guard rejects ambient secret-key credentials" \
  run_ci_roles_task_with_ambient_credential lambda-ci-roles-init AWS_SECRET_ACCESS_KEY static-secret
expect_ci_roles_rejection "CI roles guard rejects an ambient session token" \
  run_ci_roles_task_with_ambient_credential lambda-ci-roles-init AWS_SESSION_TOKEN static-session
expect_ci_roles_acceptance "CI roles guard accepts only the reviewed administrator identity" \
  'tofu -chdir=infra/lambda/ci-roles init -backend-config=backend.hcl -reconfigure -input=false' \
  run_ci_roles_as_admin lambda-ci-roles-init

ci_roles_verify_output="$tmp_dir/ci-roles-verify-output"
: > "$command_log"
run_ci_roles_as_admin lambda-ci-roles-verify > "$ci_roles_verify_output"
for ci_role_contract in \
  'AWS_RELEASE_BUILDER_ROLE_ARN=arn:aws:iam::180294223248:role/portfolio-release-builder-ci' \
  'AWS_DEVELOPMENT_DEPLOYER_ROLE_ARN=arn:aws:iam::180294223248:role/portfolio-development-deployer-ci' \
  'AWS_PRODUCTION_PLANNER_ROLE_ARN=arn:aws:iam::180294223248:role/portfolio-production-planner-ci'; do
  grep -Fxq "$ci_role_contract" "$ci_roles_verify_output" || {
    printf 'FAIL: CI role verification omitted exact variable output: %s\n' \
      "$ci_role_contract" >&2
    exit 1
  }
done
test "$(grep -Fc 'iam get-role' "$command_log")" -eq 3 || {
  echo 'FAIL: CI role verification did not read exactly three roles from IAM' >&2
  exit 1
}
pass "CI role verification reads and prints only the deterministic role ARNs"

run_ci_roles_with_wrong_role_account() (
  FAKE_ROLE_ACCOUNT=111122223333
  export FAKE_ROLE_ACCOUNT
  run_ci_roles_as_admin lambda-ci-roles-verify
)
expect_fail "CI role verification rejects an unexpected deterministic ARN" \
  run_ci_roles_with_wrong_role_account

ci_roles_identity_plan="$tmp_dir/ci-roles-identity.tfplan"
expect_ci_roles_rejection "CI roles plan rejects the normal deployer" \
  run_ci_roles_task \
  lambda-ci-roles-plan \
  portfolio-deployer \
  us-west-2 \
  180294223248 \
  arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioDeployer_abc/craig \
  PLAN_FILE="$ci_roles_identity_plan" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"

printf 'reviewed CI roles plan\n' > "$ci_roles_identity_plan"
ci_roles_identity_plan_sha256=$(shasum -a 256 "$ci_roles_identity_plan" | awk '{print $1}')
expect_ci_roles_rejection "CI roles apply rejects the normal deployer" \
  run_ci_roles_task \
  lambda-ci-roles-apply \
  portfolio-deployer \
  us-west-2 \
  180294223248 \
  arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioDeployer_abc/craig \
  PLAN_FILE="$ci_roles_identity_plan" \
  APPROVED_PLAN_SHA256="$ci_roles_identity_plan_sha256" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"

expect_pass "exact SSO identity guard" run_task lambda-dev-init
expect_fail "identity guard rejects wrong profile" \
  env PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" \
  AWS_PROFILE=default AWS_REGION=us-west-2 \
  "$real_task" --dir "$repo_root" lambda-dev-init
expect_fail "identity guard rejects wrong region" \
  env PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" \
  AWS_PROFILE=portfolio-deployer AWS_REGION=us-east-1 \
  "$real_task" --dir "$repo_root" lambda-dev-init
expect_fail "identity guard rejects ambient static credentials" \
  env PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" \
  AWS_PROFILE=portfolio-deployer AWS_REGION=us-west-2 AWS_ACCESS_KEY_ID=AKIASTATIC \
  "$real_task" --dir "$repo_root" lambda-dev-init
expect_fail "identity guard rejects root ARN" \
  env PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" \
  FAKE_ARN=arn:aws:iam::180294223248:root \
  AWS_PROFILE=portfolio-deployer AWS_REGION=us-west-2 \
  "$real_task" --dir "$repo_root" lambda-dev-init
expect_fail "identity guard rejects wrong SSO role" \
  env PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" \
  FAKE_ARN=arn:aws:sts::180294223248:assumed-role/OtherRole/craig \
  AWS_PROFILE=portfolio-deployer AWS_REGION=us-west-2 \
  "$real_task" --dir "$repo_root" lambda-dev-init

expect_pass "development backend init verifies default workspace" run_task lambda-dev-init
plan_file="$tmp_dir/dev.tfplan"
expect_fail "plan requires an absolute path" \
  run_task lambda-dev-plan \
  PLAN_FILE=relative.tfplan IMAGE_DIGEST="$release_digest" \
  APPROVED_STATE_LOCK_URI="$dev_lock_uri"
expect_fail "plan rejects the wrong lock acknowledgement" \
  run_task lambda-dev-plan \
  PLAN_FILE="$plan_file" IMAGE_DIGEST="$release_digest" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/wrong.tflock
expect_pass "development plan accepts only its exact lock acknowledgement" \
  run_task lambda-dev-plan \
  PLAN_FILE="$plan_file" IMAGE_DIGEST="$release_digest" \
  APPROVED_STATE_LOCK_URI="$dev_lock_uri"
expect_fail "plan refuses an existing saved-plan path" \
  run_task lambda-dev-plan \
  PLAN_FILE="$plan_file" IMAGE_DIGEST="$release_digest" \
  APPROVED_STATE_LOCK_URI="$dev_lock_uri"
expect_fail "apply rejects the wrong lock acknowledgement" \
  run_task lambda-dev-apply \
  PLAN_FILE="$plan_file" \
  APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/wrong.tflock
approved_plan_sha256=$(shasum -a 256 "$plan_file" | awk '{print $1}')
expect_pass "apply consumes only the checksum-bound saved plan" \
  run_task lambda-dev-apply \
  PLAN_FILE="$plan_file" APPROVED_PLAN_SHA256="$approved_plan_sha256" \
  APPROVED_STATE_LOCK_URI="$dev_lock_uri"
printf 'replacement plan bytes\n' > "$plan_file"
expect_fail "apply rejects a plan replaced at the approved path" \
  run_task lambda-dev-apply \
  PLAN_FILE="$plan_file" APPROVED_PLAN_SHA256="$approved_plan_sha256" \
  APPROVED_STATE_LOCK_URI="$dev_lock_uri"

artifact_plan_file="$tmp_dir/artifacts.tfplan"
TASK7_PLAN_JSON="$artifact_plan" expect_pass \
  "artifact plan accepts only its exact lock acknowledgement" \
  run_task lambda-artifacts-plan \
  PLAN_FILE="$artifact_plan_file" \
  APPROVED_STATE_LOCK_URI="$artifact_lock_uri"
prod_plan_file="$tmp_dir/prod.tfplan"
TASK7_PLAN_JSON="$prod_plan" expect_pass \
  "production plan accepts only its exact lock acknowledgement" \
  run_task lambda-prod-plan \
  PLAN_FILE="$prod_plan_file" IMAGE_DIGEST="$release_digest" \
  ALARM_ACTION_ARNS_JSON='["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts"]' \
  APPROVED_STATE_LOCK_URI="$prod_lock_uri"

ci_roles_plan_file="$tmp_dir/ci-roles.tfplan"
TASK7_PLAN_JSON="$ci_roles_plan" expect_ci_roles_acceptance \
  "CI role task accepts its exact contract under the administrator identity" \
  "tofu -chdir=infra/lambda/ci-roles plan -lock-timeout=5m -input=false -out=/" \
  run_ci_roles_as_admin \
  lambda-ci-roles-plan \
  PLAN_FILE="$ci_roles_plan_file" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
test -f "$ci_roles_plan_file" || {
  printf 'FAIL: CI role task removed an accepted saved plan\n' >&2
  exit 1
}
pass "CI role task retains an accepted saved plan"
ci_roles_provenance_file="$ci_roles_plan_file.provenance.json"
test -f "$ci_roles_provenance_file" || {
  printf 'FAIL: CI role task did not retain checksum-bound backend provenance\n' >&2
  exit 1
}
pass "CI role task retains checksum-bound backend provenance"
jq -e --arg plan_sha256 "$(shasum -a 256 "$ci_roles_plan_file" | awk '{print $1}')" '
  type == "object" and
  (keys | sort) == (["backend", "plan_sha256", "schema", "workspace"] | sort) and
  .schema == "portfolio.lambda-ci-roles-plan-provenance/v1" and
  .plan_sha256 == $plan_sha256 and
  .workspace == "default" and
  .backend == {
    type: "s3",
    bucket: "portfolio-tofu-state-180294223248",
    key: "portfolio-lambda-http-api/ci-roles/terraform.tfstate",
    region: "us-west-2",
    encrypt: true,
    use_lockfile: true
  }
' "$ci_roles_provenance_file" > /dev/null || {
  printf 'FAIL: CI role provenance contains unreviewed or missing fields\n' >&2
  exit 1
}
pass "CI role provenance contains only the reviewed backend contract"
init_line=$(grep -nF \
  'tofu -chdir=infra/lambda/ci-roles init -backend-config=backend.hcl -reconfigure -lockfile=readonly -input=false' \
  "$command_log" | tail -1 | cut -d: -f1)
workspace_line=$(grep -nF 'tofu -chdir=infra/lambda/ci-roles workspace show' "$command_log" | tail -1 | cut -d: -f1)
plan_line=$(grep -nF \
  'tofu -chdir=infra/lambda/ci-roles plan -lock-timeout=5m -input=false -out=/' \
  "$command_log" | tail -1 | cut -d: -f1)
test -n "$init_line" && test -n "$workspace_line" && test -n "$plan_line" &&
  test "$init_line" -lt "$workspace_line" && test "$workspace_line" -lt "$plan_line" || {
  printf 'FAIL: CI role plan was not created from a freshly initialized default workspace\n' >&2
  exit 1
}
pass "CI role task initializes exact backend and workspace immediately before planning"
grep -Eq '^tofu-cli-config /' "$command_log" || {
  printf 'FAIL: CI role plan did not force a private empty OpenTofu CLI configuration\n' >&2
  exit 1
}
pass "CI role plan forces a private empty OpenTofu CLI configuration"
if grep -Fq "tofu -chdir=infra/lambda/ci-roles show -no-color $ci_roles_plan_file" "$command_log"; then
  printf 'FAIL: CI role task rendered the caller-controlled plan path\n' >&2
  exit 1
fi
grep -Eq '^tofu -chdir=infra/lambda/ci-roles show -no-color /.*/review\.tfplan$' "$command_log" || {
  printf 'FAIL: CI role task did not render the validated private plan\n' >&2
  exit 1
}
pass "CI role task renders the validated private plan"
ci_roles_plan_sha256=$(shasum -a 256 "$ci_roles_plan_file" | awk '{print $1}')
expect_fail \
  "CI role apply rejects a plan without approved backend provenance" \
  run_ci_roles_as_admin \
  lambda-ci-roles-apply \
  PLAN_FILE="$ci_roles_plan_file" \
  APPROVED_PLAN_SHA256="$ci_roles_plan_sha256" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
ci_roles_provenance_sha256=$(shasum -a 256 "$ci_roles_provenance_file" | awk '{print $1}')
expect_ci_roles_acceptance \
  "CI role apply consumes the checksum-bound plan under the administrator identity" \
  "tofu -chdir=infra/lambda/ci-roles apply -lock-timeout=5m -input=false /" \
  run_ci_roles_as_admin \
  lambda-ci-roles-apply \
  PLAN_FILE="$ci_roles_plan_file" \
  PROVENANCE_FILE="$ci_roles_provenance_file" \
  APPROVED_PLAN_SHA256="$ci_roles_plan_sha256" \
  APPROVED_PROVENANCE_SHA256="$ci_roles_provenance_sha256" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
if grep -Fq \
  "tofu -chdir=infra/lambda/ci-roles apply -lock-timeout=5m -input=false $ci_roles_plan_file" \
  "$command_log"; then
  printf 'FAIL: CI role apply consumed the caller-controlled plan path\n' >&2
  exit 1
fi
pass "CI role apply consumes a private snapshot instead of the caller path"
grep -Eq '^tofu-cli-config /' "$command_log" || {
  printf 'FAIL: CI role apply did not force a private empty OpenTofu CLI configuration\n' >&2
  exit 1
}
pass "CI role apply forces a private empty OpenTofu CLI configuration"

: > "$command_log"
FAKE_SIGNAL_APPLY_PARENT=true expect_fail \
  "CI role apply fails when interrupted during OpenTofu apply" \
  run_ci_roles_as_admin \
  lambda-ci-roles-apply \
  PLAN_FILE="$ci_roles_plan_file" \
  PROVENANCE_FILE="$ci_roles_provenance_file" \
  APPROVED_PLAN_SHA256="$ci_roles_plan_sha256" \
  APPROVED_PROVENANCE_SHA256="$ci_roles_provenance_sha256" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
unset FAKE_SIGNAL_APPLY_PARENT
grep -Fq 'tofu -chdir=infra/lambda/ci-roles apply ' "$command_log" || {
  printf 'FAIL: CI role apply signal regression did not reach OpenTofu apply\n' >&2
  exit 1
}
pass "CI role apply propagates an interruption received during apply"

: > "$command_log"
TASK7_PLAN_JSON="$dev_plan" expect_fail \
  "CI role apply revalidates saved plan contents immediately before apply" \
  run_ci_roles_as_admin \
  lambda-ci-roles-apply \
  PLAN_FILE="$ci_roles_plan_file" \
  PROVENANCE_FILE="$ci_roles_provenance_file" \
  APPROVED_PLAN_SHA256="$ci_roles_plan_sha256" \
  APPROVED_PROVENANCE_SHA256="$ci_roles_provenance_sha256" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
if grep -Fq 'tofu -chdir=infra/lambda/ci-roles apply ' "$command_log"; then
  printf 'FAIL: CI role apply reached apply after rejecting plan contents\n' >&2
  exit 1
fi
pass "CI role apply stops before apply when saved plan contents fail policy"

ci_roles_wrong_backend_provenance="$tmp_dir/ci-roles-wrong-backend.provenance.json"
jq '.backend.bucket = "unreviewed-state-bucket"' \
  "$ci_roles_provenance_file" > "$ci_roles_wrong_backend_provenance"
wrong_backend_provenance_sha256=$(shasum -a 256 "$ci_roles_wrong_backend_provenance" | awk '{print $1}')
expect_ci_roles_rejection \
  "CI role apply rejects approved provenance for the wrong backend" \
  run_ci_roles_as_admin \
  lambda-ci-roles-apply \
  PLAN_FILE="$ci_roles_plan_file" \
  PROVENANCE_FILE="$ci_roles_wrong_backend_provenance" \
  APPROVED_PLAN_SHA256="$ci_roles_plan_sha256" \
  APPROVED_PROVENANCE_SHA256="$wrong_backend_provenance_sha256" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"

ci_roles_override_plan="$tmp_dir/ci-roles-override.tfplan"
expect_ci_roles_rejection \
  "CI role plan rejects an ambient workspace override" \
  run_ci_roles_task_with_ambient_credential \
  lambda-ci-roles-plan \
  TF_WORKSPACE \
  unreviewed \
  PLAN_FILE="$ci_roles_override_plan" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
expect_ci_roles_rejection \
  "CI role apply rejects an ambient lock-disabling CLI override" \
  run_ci_roles_task_with_ambient_credential \
  lambda-ci-roles-apply \
  TF_CLI_ARGS_apply \
  -lock=false \
  PLAN_FILE="$ci_roles_plan_file" \
  PROVENANCE_FILE="$ci_roles_provenance_file" \
  APPROVED_PLAN_SHA256="$ci_roles_plan_sha256" \
  APPROVED_PROVENANCE_SHA256="$ci_roles_provenance_sha256" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
expect_ci_roles_rejection \
  "CI role plan rejects an ambient provider reattachment" \
  run_ci_roles_task_with_ambient_credential \
  lambda-ci-roles-plan \
  TF_REATTACH_PROVIDERS \
  '{}' \
  PLAN_FILE="$ci_roles_override_plan" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
expect_ci_roles_rejection \
  "CI role plan rejects an ambient OpenTofu CLI configuration" \
  run_ci_roles_task_with_ambient_credential \
  lambda-ci-roles-plan \
  TF_CLI_CONFIG_FILE \
  /tmp/unreviewed-tofurc \
  PLAN_FILE="$ci_roles_override_plan" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
expect_ci_roles_rejection \
  "CI role apply rejects an ambient S3 endpoint override" \
  run_ci_roles_task_with_ambient_credential \
  lambda-ci-roles-apply \
  AWS_ENDPOINT_URL_S3 \
  https://attacker.example \
  PLAN_FILE="$ci_roles_plan_file" \
  PROVENANCE_FILE="$ci_roles_provenance_file" \
  APPROVED_PLAN_SHA256="$ci_roles_plan_sha256" \
  APPROVED_PROVENANCE_SHA256="$ci_roles_provenance_sha256" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
expect_ci_roles_rejection \
  "CI role plan rejects an ambient S3 customer encryption key" \
  run_ci_roles_task_with_ambient_credential \
  lambda-ci-roles-plan \
  AWS_SSE_CUSTOMER_KEY \
  AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA= \
  PLAN_FILE="$ci_roles_override_plan" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
expect_ci_roles_rejection \
  "CI role apply rejects an ambient DynamoDB endpoint override" \
  run_ci_roles_task_with_ambient_credential \
  lambda-ci-roles-apply \
  AWS_ENDPOINT_URL_DYNAMODB \
  https://attacker.example \
  PLAN_FILE="$ci_roles_plan_file" \
  PROVENANCE_FILE="$ci_roles_provenance_file" \
  APPROVED_PLAN_SHA256="$ci_roles_plan_sha256" \
  APPROVED_PROVENANCE_SHA256="$ci_roles_provenance_sha256" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
expect_ci_roles_rejection \
  "CI role plan rejects the legacy ambient DynamoDB endpoint override" \
  run_ci_roles_task_with_ambient_credential \
  lambda-ci-roles-plan \
  AWS_DYNAMODB_ENDPOINT \
  https://attacker.example \
  PLAN_FILE="$ci_roles_override_plan" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
expect_ci_roles_rejection \
  "CI role plan rejects ambient OpenTofu encryption configuration" \
  run_ci_roles_task_with_ambient_credential \
  lambda-ci-roles-plan \
  TF_ENCRYPTION \
  unreviewed \
  PLAN_FILE="$ci_roles_override_plan" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
expect_ci_roles_rejection \
  "CI role apply rejects ambient OpenTofu encryption configuration" \
  run_ci_roles_task_with_ambient_credential \
  lambda-ci-roles-apply \
  TF_ENCRYPTION \
  unreviewed \
  PLAN_FILE="$ci_roles_plan_file" \
  PROVENANCE_FILE="$ci_roles_provenance_file" \
  APPROVED_PLAN_SHA256="$ci_roles_plan_sha256" \
  APPROVED_PROVENANCE_SHA256="$ci_roles_provenance_sha256" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"

ci_roles_extra_backend_plan="$tmp_dir/ci-roles-extra-backend.tfplan"
TASK7_PLAN_JSON="$ci_roles_plan" \
  FAKE_BACKEND_EXTRA_JSON='{"acl":"public-read"}' expect_fail \
  "CI role plan rejects an unreviewed state-object ACL" \
  run_ci_roles_as_admin \
  lambda-ci-roles-plan \
  PLAN_FILE="$ci_roles_extra_backend_plan" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
test ! -e "$ci_roles_extra_backend_plan" || {
  printf 'FAIL: CI role plan retained a plan with an unreviewed state-object ACL\n' >&2
  exit 1
}
pass "CI role plan removes artifacts with an unreviewed state-object ACL"

ci_roles_sse_backend_plan="$tmp_dir/ci-roles-sse-backend.tfplan"
TASK7_PLAN_JSON="$ci_roles_plan" \
  FAKE_BACKEND_EXTRA_JSON='{"sse_customer_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}' expect_fail \
  "CI role plan rejects an unreviewed state-object customer encryption key" \
  run_ci_roles_as_admin \
  lambda-ci-roles-plan \
  PLAN_FILE="$ci_roles_sse_backend_plan" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
test ! -e "$ci_roles_sse_backend_plan" && test ! -e "$ci_roles_sse_backend_plan.provenance.json" || {
  printf 'FAIL: CI role plan retained artifacts with an unreviewed customer encryption key\n' >&2
  exit 1
}
pass "CI role plan removes artifacts with an unreviewed customer encryption key"

ci_roles_kms_backend_plan="$tmp_dir/ci-roles-kms-backend.tfplan"
unreviewed_kms_key_arn=arn:aws:kms:us-west-2:180294223248:key/00000000-0000-0000-0000-000000000000
unreviewed_kms_backend=$(jq -nc \
  --arg kms_key_id "$unreviewed_kms_key_arn" \
  '{kms_key_id:$kms_key_id}')
TASK7_PLAN_JSON="$ci_roles_plan" \
  FAKE_BACKEND_EXTRA_JSON="$unreviewed_kms_backend" expect_fail \
  "CI role plan rejects an unreviewed state KMS key" \
  run_ci_roles_as_admin \
  lambda-ci-roles-plan \
  PLAN_FILE="$ci_roles_kms_backend_plan" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"

ci_roles_dynamodb_backend_plan="$tmp_dir/ci-roles-dynamodb-backend.tfplan"
TASK7_PLAN_JSON="$ci_roles_plan" \
  FAKE_BACKEND_EXTRA_JSON='{"dynamodb_table":"unreviewed-locks"}' expect_fail \
  "CI role plan rejects an unreviewed DynamoDB lock table" \
  run_ci_roles_as_admin \
  lambda-ci-roles-plan \
  PLAN_FILE="$ci_roles_dynamodb_backend_plan" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
TASK7_PLAN_JSON="$ci_roles_plan" \
  FAKE_BACKEND_EXTRA_JSON='{"dynamodb_endpoint":"https://attacker.example"}' expect_fail \
  "CI role plan rejects an unreviewed DynamoDB backend endpoint" \
  run_ci_roles_as_admin \
  lambda-ci-roles-plan \
  PLAN_FILE="$ci_roles_dynamodb_backend_plan" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"

TASK7_PLAN_JSON="$ci_roles_plan" \
  FAKE_BACKEND_EXTRA_JSON='{"profile":"unreviewed-profile"}' expect_fail \
  "CI role plan rejects security-sensitive backend settings outside the provenance" \
  run_ci_roles_as_admin \
  lambda-ci-roles-plan \
  PLAN_FILE="$ci_roles_extra_backend_plan" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
test ! -e "$ci_roles_extra_backend_plan" || {
  printf 'FAIL: CI role plan retained a plan with unreviewed backend settings\n' >&2
  exit 1
}
pass "CI role plan removes artifacts with unreviewed backend settings"

ci_roles_publish_plan="$tmp_dir/ci-roles-publish.tfplan"
: > "$command_log"
TASK7_PLAN_JSON="$ci_roles_plan" \
  FAKE_BACKEND_EXTRA_JSON='' \
  FAKE_MUTATE_PLAN_PATH="$ci_roles_publish_plan" expect_fail \
  "CI role plan fails closed when the published plan changes during publication" \
  run_ci_roles_as_admin \
  lambda-ci-roles-plan \
  PLAN_FILE="$ci_roles_publish_plan" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
test ! -e "$ci_roles_publish_plan" && test ! -e "$ci_roles_publish_plan.provenance.json" || {
  printf 'FAIL: CI role plan retained artifacts after a publication integrity failure\n' >&2
  exit 1
}
if grep -Fq "tofu -chdir=infra/lambda/ci-roles show -no-color $ci_roles_publish_plan" "$command_log"; then
  printf 'FAIL: CI role plan rendered the caller-controlled publication path\n' >&2
  exit 1
fi
grep -Eq '^tofu -chdir=infra/lambda/ci-roles show -no-color /.*/review\.tfplan$' "$command_log" || {
  printf 'FAIL: CI role plan did not render its validated private plan before publication\n' >&2
  exit 1
}
pass "CI role plan removes publication artifacts that fail integrity verification"

ci_roles_bad_plan_file="$tmp_dir/ci-roles-bad.tfplan"
TASK7_PLAN_JSON="$dev_plan" \
  FAKE_BACKEND_EXTRA_JSON='' expect_fail \
  "CI role task rejects resources outside its exact contract" \
  run_ci_roles_as_admin \
  lambda-ci-roles-plan \
  PLAN_FILE="$ci_roles_bad_plan_file" \
  APPROVED_STATE_LOCK_URI="$ci_roles_lock_uri"
test ! -e "$ci_roles_bad_plan_file" || {
  printf 'FAIL: CI role task retained a rejected saved plan\n' >&2
  exit 1
}
pass "CI role task removes a rejected saved plan"

ci_roles_interrupted_plan_file="$tmp_dir/ci-roles-interrupted.tfplan"
unset FAKE_BACKEND_EXTRA_JSON
if PATH="$fake_bin:$PATH" \
  COMMAND_LOG="$command_log" \
  AWS_PROFILE=portfolio-ci-roles-administrator \
  AWS_REGION=us-west-2 \
  FAKE_PLAN_JSON="$ci_roles_plan" \
  FAKE_TOFU_SIGNAL_PARENT=true \
  PLAN_FILE="$ci_roles_interrupted_plan_file" \
  sh "$repo_root/scripts/create-ci-roles-plan.sh"; then
  printf 'FAIL: CI role task accepted a signal-interrupted plan\n' >&2
  exit 1
fi
test ! -e "$ci_roles_interrupted_plan_file" || {
  printf 'FAIL: CI role task retained a signal-interrupted plan\n' >&2
  exit 1
}
pass "CI role task removes a signal-interrupted saved plan"

expect_pass "immutable full-SHA release push" run_task lambda-release-push
expected_scan_wait="ecr wait image-scan-complete --repository-name portfolio-lambda-releases"
grep -F "$expected_scan_wait --image-id imageTag=$release_tag" "$command_log" > /dev/null || {
  printf 'FAIL: release did not wait for the current ECR scan to complete\n' >&2
  exit 1
}
pass "release waits for the current ECR scan"
expected_scan_read="ecr describe-image-scan-findings --repository-name portfolio-lambda-releases"
grep -F "$expected_scan_read --image-id imageTag=$release_tag" "$command_log" > /dev/null || {
  printf 'FAIL: release did not retrieve findings through the current ECR scan API\n' >&2
  exit 1
}
pass "release uses the current ECR scan findings API"
scan_metadata_calls=$(grep ' ecr describe-image-scan-findings ' "$command_log")
if [ -z "$scan_metadata_calls" ] ||
  printf '%s\n' "$scan_metadata_calls" | grep -v -- ' --no-paginate ' > /dev/null; then
  printf 'FAIL: release allowed pagination while reading ECR scan metadata\n' >&2
  exit 1
fi
pass "release disables pagination for ECR scan metadata reads"
sleeps_before=$(grep -c '^sleep 5$' "$command_log" || true)
FAKE_WAITER_MODE=complete FAKE_SCAN_MODE=missing-once \
  expect_pass "release tolerates one initial missing scan record" \
  run_task lambda-release-push
sleeps_after=$(grep -c '^sleep 5$' "$command_log" || true)
test "$((sleeps_after - sleeps_before))" -eq 1 || {
  printf 'FAIL: release did not retry a one-time missing scan exactly once\n' >&2
  exit 1
}
pass "release retries a one-time missing scan exactly once"
sleeps_before=$sleeps_after
FAKE_WAITER_MODE=complete FAKE_SCAN_MODE=missing \
  expect_fail "release bounds a persistently missing scan record" \
  run_task lambda-release-push
sleeps_after=$(grep -c '^sleep 5$' "$command_log" || true)
test "$((sleeps_after - sleeps_before))" -eq 11 || {
  printf 'FAIL: release did not use the exact bounded scan-discovery wait\n' >&2
  exit 1
}
pass "release uses exactly eleven bounded scan-discovery sleeps"
FAKE_WAITER_MODE=denied FAKE_SCAN_MODE=complete \
  expect_fail "release fails closed when the scan waiter is denied" \
  run_task lambda-release-push
FAKE_WAITER_MODE=failed FAKE_SCAN_MODE=complete \
  expect_fail "release fails closed when the scan waiter reports failure" \
  run_task lambda-release-push
sleeps_before=$(grep -c '^sleep 5$' "$command_log" || true)
scan_lookups_before=$(grep -c ' ecr describe-image-scan-findings ' "$command_log" || true)
FAKE_WAITER_MODE=complete FAKE_SCAN_MODE=denied \
  expect_fail "release fails closed when scan findings are unreadable" \
  run_task lambda-release-push
sleeps_after=$(grep -c '^sleep 5$' "$command_log" || true)
scan_lookups_after=$(grep -c ' ecr describe-image-scan-findings ' "$command_log" || true)
test "$((sleeps_after - sleeps_before))" -eq 0 && test "$((scan_lookups_after - scan_lookups_before))" -eq 1 || {
  printf 'FAIL: release did not fail immediately on a denied scan lookup\n' >&2
  exit 1
}
pass "release performs one lookup and no sleep after scan denial"
sleeps_before=$sleeps_after
scan_lookups_before=$scan_lookups_after
FAKE_WAITER_MODE=complete FAKE_SCAN_MODE=ambiguous \
  expect_fail "release rejects an ambiguous scan lookup error" \
  run_task lambda-release-push
sleeps_after=$(grep -c '^sleep 5$' "$command_log" || true)
scan_lookups_after=$(grep -c ' ecr describe-image-scan-findings ' "$command_log" || true)
test "$((sleeps_after - sleeps_before))" -eq 0 && test "$((scan_lookups_after - scan_lookups_before))" -eq 1 || {
  printf 'FAIL: release retried an ambiguous scan lookup error\n' >&2
  exit 1
}
pass "release performs one lookup and no sleep for ambiguous scan errors"
FAKE_WAITER_MODE=complete FAKE_SCAN_MODE=failed \
  expect_fail "release fails closed when the findings status is not complete" \
  run_task lambda-release-push
pushes_before=$(grep -c '^docker push ' "$command_log" || true)
FAKE_LOOKUP_MODE=denied expect_fail "release push fails closed on tag lookup denial" run_task lambda-release-push
pushes_after=$(grep -c '^docker push ' "$command_log" || true)
test "$pushes_after" = "$pushes_before" || {
  printf 'FAIL: release pushed after a denied tag lookup\n' >&2
  exit 1
}
pass "release performs no push after a denied tag lookup"
FAKE_LOOKUP_MODE=ambiguous expect_fail "release accepts only the exact tag-absence error" run_task lambda-release-push
FAKE_REPOSITORY_MUTABILITY=MUTABLE expect_fail \
  "release push requires an immutable repository" \
  run_task lambda-release-push
pushes_before=$(grep -c '^docker push ' "$command_log" || true)
FAKE_LOOKUP_MODE=existing expect_fail "release stops when the immutable tag already exists" run_task lambda-release-push
pushes_after=$(grep -c '^docker push ' "$command_log" || true)
test "$pushes_after" = "$pushes_before" || {
  printf 'FAIL: release pushed an existing immutable tag\n' >&2
  exit 1
}
pass "release performs no push for an existing immutable tag"
FAKE_PUSH_FAIL=true expect_fail "release push propagates registry conflicts" run_task lambda-release-push
expected_build_task="task build-lambda-image BUILD_REVISION=$release_source_sha"
grep -F "$expected_build_task IMAGE_TAG=$release_tagged_image" "$command_log" > /dev/null || {
  printf 'FAIL: release build did not receive the immutable full-SHA tag\n' >&2
  exit 1
}
grep -F "docker push $release_tagged_image" "$command_log" > /dev/null || {
  printf 'FAIL: release did not push the immutable full-SHA tag\n' >&2
  exit 1
}
pass "release command used the immutable full-SHA tag"

if grep -En -- '--auto-approve|-target=|:latest|lambda-latest' "$command_log" > /dev/null; then
  printf 'FAIL: replacement command execution used an unsafe plan or mutable tag\n' >&2
  exit 1
fi
pass "replacement command executions used no auto-approve, target, or mutable tag"

printf 'PASS: %s Lambda plan contracts\n' "$pass_count"
