package bootstrap

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

const appRunnerRetirementPolicyPath = "portfolio-deployer-app-runner-retirement-policy.json"

func TestAppRunnerRetirementPolicyHasExactTemporaryScope(t *testing.T) {
	data, policy := loadPolicy(t, appRunnerRetirementPolicyPath)
	if policy.Version != "2012-10-17" {
		t.Fatalf("retirement policy version = %q, want 2012-10-17", policy.Version)
	}

	expected := map[string]struct {
		actions    map[string]struct{}
		resources  map[string]struct{}
		conditions map[string]any
	}{
		"CallerIdentity": {
			actions:   stringSet("sts:GetCallerIdentity"),
			resources: stringSet("*"),
		},
		"LegacyStateBucketLocation": {
			actions:   stringSet("s3:GetBucketLocation"),
			resources: stringSet("arn:aws:s3:::portfolio-tofu-state-180294223248"),
		},
		"LegacyStateList": {
			actions:   stringSet("s3:ListBucket"),
			resources: stringSet("arn:aws:s3:::portfolio-tofu-state-180294223248"),
			conditions: map[string]any{
				"StringEquals": map[string]any{
					"s3:prefix": []any{
						"env:/",
						"portfolio/terraform.tfstate",
						"portfolio/terraform.tfstate.tflock",
					},
				},
			},
		},
		"LegacyStateObject": {
			actions: stringSet("s3:GetObject", "s3:PutObject"),
			resources: stringSet(
				"arn:aws:s3:::portfolio-tofu-state-180294223248/portfolio/terraform.tfstate",
			),
		},
		"LegacyStateLock": {
			actions: stringSet("s3:DeleteObject", "s3:GetObject", "s3:PutObject"),
			resources: stringSet(
				"arn:aws:s3:::portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock",
			),
		},
		"LegacyKMSAliasList": {
			actions:    stringSet("kms:ListAliases"),
			resources:  stringSet("*"),
			conditions: requestedRegionCondition(),
		},
		"LegacyECRRead": {
			actions: stringSet(
				"ecr:DescribeRepositories",
				"ecr:GetLifecyclePolicy",
				"ecr:ListTagsForResource",
			),
			resources:  stringSet("arn:aws:ecr:us-west-2:180294223248:repository/portfolio"),
			conditions: requestedRegionCondition(),
		},
		"LegacyDynamoDBRead": {
			actions: stringSet(
				"dynamodb:DescribeContinuousBackups",
				"dynamodb:DescribeTable",
				"dynamodb:DescribeTimeToLive",
				"dynamodb:ListTagsOfResource",
			),
			resources: stringSet(
				"arn:aws:dynamodb:us-west-2:180294223248:table/portfolio-google-connections",
				"arn:aws:dynamodb:us-west-2:180294223248:table/portfolio-soccer-sessions",
			),
			conditions: requestedRegionCondition(),
		},
		"LegacyRoleRead": {
			actions: stringSet(
				"iam:GetRole",
				"iam:GetRolePolicy",
				"iam:ListAttachedRolePolicies",
				"iam:ListRolePolicies",
				"iam:ListRoleTags",
			),
			resources: stringSet(
				"arn:aws:iam::180294223248:role/portfolio-apprunner-ecr-access",
				"arn:aws:iam::180294223248:role/portfolio-apprunner-instance",
			),
		},
		"LegacyManagedPolicyRead": {
			actions: stringSet(
				"iam:GetPolicy",
				"iam:GetPolicyVersion",
				"iam:ListPolicyTags",
			),
			resources: stringSet(
				"arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets",
				"arn:aws:iam::180294223248:policy/portfolio-google-connections-dynamodb",
				"arn:aws:iam::180294223248:policy/portfolio-soccer-sessions-dynamodb",
			),
		},
		"LegacyAppRunnerReadDelete": {
			actions: stringSet(
				"apprunner:DeleteService",
				"apprunner:DescribeCustomDomains",
				"apprunner:DescribeService",
				"apprunner:ListTagsForResource",
			),
			resources: stringSet(
				"arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb",
			),
			conditions: requestedRegionCondition(),
		},
		"RetireAppRunnerECRAccessAttachment": {
			actions: stringSet("iam:DetachRolePolicy"),
			resources: stringSet(
				"arn:aws:iam::180294223248:role/portfolio-apprunner-ecr-access",
			),
			conditions: map[string]any{
				"ArnEquals": map[string]any{
					"iam:PolicyARN": "arn:aws:iam::aws:policy/service-role/AWSAppRunnerServicePolicyForECRAccess",
				},
			},
		},
		"RetireAppRunnerInstanceAttachments": {
			actions: stringSet("iam:DetachRolePolicy"),
			resources: stringSet(
				"arn:aws:iam::180294223248:role/portfolio-apprunner-instance",
			),
			conditions: map[string]any{
				"ArnEquals": map[string]any{
					"iam:PolicyARN": []any{
						"arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets",
						"arn:aws:iam::180294223248:policy/portfolio-google-connections-dynamodb",
						"arn:aws:iam::180294223248:policy/portfolio-soccer-sessions-dynamodb",
					},
				},
			},
		},
		"RetireAppRunnerRoles": {
			actions: stringSet("iam:DeleteRole", "iam:ListInstanceProfilesForRole"),
			resources: stringSet(
				"arn:aws:iam::180294223248:role/portfolio-apprunner-ecr-access",
				"arn:aws:iam::180294223248:role/portfolio-apprunner-instance",
			),
		},
		"RetireAppRunnerRuntimePolicy": {
			actions: stringSet(
				"iam:DeletePolicy",
				"iam:ListEntitiesForPolicy",
				"iam:ListPolicyVersions",
			),
			resources: stringSet(
				"arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets",
			),
		},
	}

	if got := nonWhitespaceByteCount(data); got > identityCenterNonWhitespaceLimit {
		t.Fatalf("retirement policy size %d exceeds Identity Center limit %d", got, identityCenterNonWhitespaceLimit)
	}

	kmsKeyPattern := regexp.MustCompile(`^arn:aws:kms:us-west-2:180294223248:key/[0-9a-f-]+$`)
	found := make(map[string]int, len(expected)+1)
	for _, statement := range policy.Statement {
		if statement.Sid == "LegacyKMSKeyRead" {
			found[statement.Sid]++
			if statement.Effect != "Allow" {
				t.Errorf("legacy KMS key effect = %q, want Allow", statement.Effect)
			}
			assertStringSet(t, "legacy KMS key actions", stringSet(statement.Action...), stringSet("kms:DescribeKey"))
			if len(statement.Resource) != 1 || !kmsKeyPattern.MatchString(statement.Resource[0]) {
				t.Errorf("legacy KMS key resources = %v, want one captured key ARN", statement.Resource)
			}
			if !reflect.DeepEqual(statement.Condition, requestedRegionCondition()) {
				t.Errorf("legacy KMS key conditions = %#v, want %#v", statement.Condition, requestedRegionCondition())
			}
			continue
		}

		want, ok := expected[statement.Sid]
		if !ok {
			t.Errorf("unreviewed retirement policy statement %q", statement.Sid)
			continue
		}
		found[statement.Sid]++
		if statement.Effect != "Allow" {
			t.Errorf("statement %q effect = %q, want Allow", statement.Sid, statement.Effect)
		}
		assertStringSet(t, "actions for "+statement.Sid, stringSet(statement.Action...), want.actions)
		assertStringSet(t, "resources for "+statement.Sid, stringSet(statement.Resource...), want.resources)
		if !reflect.DeepEqual(statement.Condition, want.conditions) {
			t.Errorf("conditions for %q = %#v, want %#v", statement.Sid, statement.Condition, want.conditions)
		}
	}

	wantSIDs := stringSetFromKeys(expected)
	wantSIDs["LegacyKMSKeyRead"] = struct{}{}
	assertStringSet(t, "retirement policy Sids", stringSetFromKeys(found), wantSIDs)
	for sid := range wantSIDs {
		if found[sid] != 1 {
			t.Errorf("retirement policy statement %q count = %d, want 1", sid, found[sid])
		}
	}

	lowerPolicy := strings.ToLower(string(data))
	for _, forbidden := range []string{
		"portfolio-lambda-http-api/",
		"portfolio-lambda-dev",
		"portfolio-lambda-prod",
		"portfolio-lambda-execution",
		"portfolio-lambda-runtime-secrets",
		"disassociatecustomdomain",
		"iam:attach",
		"iam:create",
		"iam:passrole",
		"iam:put",
		"apprunner:createservice",
		"apprunner:updateservice",
		"dynamodb:delete",
		"ecr:delete",
		"lambda:",
		"apigateway:",
		"sso:",
		"identitystore:",
	} {
		if strings.Contains(lowerPolicy, forbidden) {
			t.Errorf("retirement policy contains forbidden scope %q", forbidden)
		}
	}
}

func TestAppRunnerRetirementPolicyListsOnlyExactLegacyStateKeys(t *testing.T) {
	_, policy := loadPolicy(t, appRunnerRetirementPolicyPath)

	for _, statement := range policy.Statement {
		if statement.Sid != "LegacyStateList" {
			continue
		}
		if _, ok := statement.Condition["NumericLessThanEquals"]; ok {
			t.Fatal("legacy state discovery must not use s3:max-keys conditions")
		}
		stringEquals, ok := statement.Condition["StringEquals"].(map[string]any)
		if !ok {
			t.Fatal("legacy state list statement lacks StringEquals conditions")
		}
		prefixes, ok := stringEquals["s3:prefix"].([]any)
		if !ok {
			t.Fatalf("legacy state prefixes have type %T, want array", stringEquals["s3:prefix"])
		}
		got := make(map[string]struct{}, len(prefixes))
		for _, item := range prefixes {
			prefix, ok := item.(string)
			if !ok {
				t.Fatalf("legacy state prefix has type %T, want string", item)
			}
			got[prefix] = struct{}{}
		}
		assertStringSet(t, "legacy state ListBucket prefixes", got, stringSet(
			"env:/",
			"portfolio/terraform.tfstate",
			"portfolio/terraform.tfstate.tflock",
		))
		return
	}

	t.Fatal("legacy state list statement not found")
}
