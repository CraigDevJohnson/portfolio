package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

const identityCenterNonWhitespaceLimit = 10_240

type stringList []string

func (values *stringList) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*values = []string{single}
		return nil
	}

	var multiple []string
	if err := json.Unmarshal(data, &multiple); err != nil {
		return fmt.Errorf("decode string or string list: %w", err)
	}
	*values = multiple
	return nil
}

type policyStatement struct {
	Sid       string         `json:"Sid"`
	Effect    string         `json:"Effect"`
	Action    stringList     `json:"Action"`
	Resource  stringList     `json:"Resource"`
	Condition map[string]any `json:"Condition"`
}

type policyDocument struct {
	Version   string            `json:"Version"`
	Statement []policyStatement `json:"Statement"`
}

func TestReviewedPolicyFilesMatchApprovedArtifacts(t *testing.T) {
	tests := []struct {
		name               string
		path               string
		sha256             string
		bytes              int
		nonWhitespaceBytes int
		identityCenter     bool
	}{
		{
			name:               "development bootstrap",
			path:               "portfolio-deployer-development-bootstrap-policy.json",
			sha256:             "b5236b3201232e1af97109d5eab6f514990dcfdb77336c867aff5bcc25b1bba4",
			bytes:              13_535,
			nonWhitespaceBytes: 10_098,
			identityCenter:     true,
		},
		{
			name:               "execution boundary",
			path:               "portfolio-lambda-execution-boundary-policy.json",
			sha256:             "540e803575f8235e3b6fec64900371cb7fb1556de7094ce8e97060403dd191f5",
			bytes:              5_510,
			nonWhitespaceBytes: 4_159,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, _ := loadPolicy(t, test.path)
			digest := sha256.Sum256(data)
			if got := hex.EncodeToString(digest[:]); got != test.sha256 {
				t.Errorf("SHA-256 = %s, want reviewed digest %s", got, test.sha256)
			}
			if got := len(data); got != test.bytes {
				t.Errorf("byte count = %d, want %d", got, test.bytes)
			}
			nonWhitespaceBytes := nonWhitespaceByteCount(data)
			if nonWhitespaceBytes != test.nonWhitespaceBytes {
				t.Errorf("non-whitespace byte count = %d, want %d", nonWhitespaceBytes, test.nonWhitespaceBytes)
			}
			if test.identityCenter && nonWhitespaceBytes > identityCenterNonWhitespaceLimit {
				t.Errorf("reviewed policy size %d exceeds Identity Center limit %d", nonWhitespaceBytes, identityCenterNonWhitespaceLimit)
			}
		})
	}
}

func TestDevelopmentBootstrapPolicyKeepsReviewedScope(t *testing.T) {
	data, policy := loadPolicy(t, "portfolio-deployer-development-bootstrap-policy.json")

	expectedControlledSIDs := stringSet(
		"D", "P", "T4", "T5", "T8", "T9", "TA", "TB",
		"TD", "TE", "TF", "TG", "TH", "TI", "TK", "TM", "TN",
	)
	controlledSIDs := make(map[string]struct{})
	for _, statement := range policy.Statement {
		if statement.Sid == "" {
			continue
		}
		if _, duplicate := controlledSIDs[statement.Sid]; duplicate {
			t.Errorf("duplicate controlled Sid %q", statement.Sid)
		}
		controlledSIDs[statement.Sid] = struct{}{}
	}
	assertStringSet(t, "controlled bootstrap Sids", controlledSIDs, expectedControlledSIDs)

	lowerPolicy := strings.ToLower(string(data))
	for _, forbidden := range []string{
		"prod",
		"apprunner:",
		"amplify:",
		"ec2:",
		"route53:",
		"organizations:",
		"sso:",
		"identitystore:",
	} {
		if strings.Contains(lowerPolicy, forbidden) {
			t.Errorf("development bootstrap policy contains forbidden scope %q", forbidden)
		}
	}

	allowedDeleteActions := stringSet("logs:DeleteLogDelivery", "s3:DeleteObject")
	allowedIAMActions := stringSet(
		"iam:CreateRole",
		"iam:GetRole",
		"iam:GetRolePolicy",
		"iam:ListAttachedRolePolicies",
		"iam:ListRolePolicies",
		"iam:ListRoleTags",
		"iam:PassRole",
		"iam:PutRolePolicy",
		"iam:TagRole",
	)
	for _, statement := range policy.Statement {
		for _, action := range statement.Action {
			if action == "*" || strings.HasSuffix(action, ":*") {
				t.Errorf("statement %q has wildcard action %q", statement.Sid, action)
			}
			if strings.Contains(strings.ToLower(action), ":delete") {
				if _, ok := allowedDeleteActions[action]; !ok {
					t.Errorf("statement %q has unreviewed destructive action %q", statement.Sid, action)
				}
			}
			if strings.HasPrefix(action, "iam:") {
				if _, ok := allowedIAMActions[action]; !ok {
					t.Errorf("statement %q has unreviewed IAM administration action %q", statement.Sid, action)
				}
			}
		}
	}

	legacyStatementActions := map[string]map[string]struct{}{
		"TD": stringSet("ssm:GetParameter"),
		"TG": stringSet("kms:Decrypt"),
	}
	gotLegacySIDs := make(map[string]struct{})
	for _, statement := range policy.Statement {
		encoded, err := json.Marshal(statement)
		if err != nil {
			t.Fatalf("marshal statement %q: %v", statement.Sid, err)
		}
		if !containsLegacySource(string(encoded)) {
			continue
		}
		gotLegacySIDs[statement.Sid] = struct{}{}
		allowedActions, ok := legacyStatementActions[statement.Sid]
		if !ok {
			t.Errorf("statement %q reaches an unreviewed legacy source", statement.Sid)
			continue
		}
		assertStringSet(t, "legacy actions for "+statement.Sid, stringSet(statement.Action...), allowedActions)
	}
	assertStringSet(t, "legacy source Sids", gotLegacySIDs, stringSet("TD", "TG"))
}

func TestDevelopmentBootstrapPolicyListsOnlyReviewedStatePrefixes(t *testing.T) {
	_, policy := loadPolicy(t, "portfolio-deployer-development-bootstrap-policy.json")

	var prefixes map[string]struct{}
	for _, statement := range policy.Statement {
		if !stringSetContains(stringSet(statement.Action...), "s3:ListBucket") {
			continue
		}
		if !stringSetContains(stringSet(statement.Resource...), "arn:aws:s3:::portfolio-tofu-state-180294223248") {
			continue
		}

		stringEquals, ok := statement.Condition["StringEquals"].(map[string]any)
		if !ok {
			t.Fatalf("state bucket ListBucket statement lacks StringEquals conditions")
		}
		switch value := stringEquals["s3:prefix"].(type) {
		case string:
			prefixes = stringSet(value)
		case []any:
			prefixes = make(map[string]struct{}, len(value))
			for _, item := range value {
				prefix, ok := item.(string)
				if !ok {
					t.Fatalf("state bucket ListBucket prefix contains non-string %T", item)
				}
				prefixes[prefix] = struct{}{}
			}
		default:
			t.Fatalf("state bucket ListBucket prefix has unsupported type %T", value)
		}
	}

	assertStringSet(t, "reviewed state ListBucket prefixes", prefixes, stringSet(
		"env:/",
		"portfolio-lambda-http-api/artifacts/terraform.tfstate",
		"portfolio-lambda-http-api/dev/terraform.tfstate",
	))
}

func TestDevelopmentBootstrapPolicyAllowsMissingStateDiscoveryWithoutMaxKeys(t *testing.T) {
	_, policy := loadPolicy(t, "portfolio-deployer-development-bootstrap-policy.json")

	found := false
	for _, statement := range policy.Statement {
		if !stringSetContains(stringSet(statement.Action...), "s3:ListBucket") {
			continue
		}
		if !stringSetContains(stringSet(statement.Resource...), "arn:aws:s3:::portfolio-tofu-state-180294223248") {
			continue
		}

		found = true
		if numeric, ok := statement.Condition["NumericLessThanEquals"].(map[string]any); ok {
			if _, gated := numeric["s3:max-keys"]; gated {
				t.Fatal("state discovery is gated by s3:max-keys, which HeadObject does not supply for a missing object")
			}
		}
	}

	if !found {
		t.Fatal("state bucket ListBucket statement not found")
	}
}

func TestDevelopmentBootstrapPolicyCannotChangeBucketVersioningAfterGate(t *testing.T) {
	_, policy := loadPolicy(t, "portfolio-deployer-development-bootstrap-policy.json")

	for _, statement := range policy.Statement {
		for _, action := range statement.Action {
			if action == "s3:PutBucketVersioning" {
				t.Fatalf("statement %q retains the consumed bucket-versioning permission", statement.Sid)
			}
		}
	}
}

func TestDevelopmentBootstrapPolicyCannotAdministerArtifactsAfterConvergence(t *testing.T) {
	_, policy := loadPolicy(t, "portfolio-deployer-development-bootstrap-policy.json")

	consumedActions := stringSet(
		"ecr:CreateRepository",
		"ecr:PutImageScanningConfiguration",
		"ecr:PutImageTagMutability",
		"ecr:PutLifecyclePolicy",
		"ecr:SetRepositoryPolicy",
		"ecr:TagResource",
	)
	for _, statement := range policy.Statement {
		for _, action := range statement.Action {
			if stringSetContains(consumedActions, action) {
				t.Fatalf("statement %q retains consumed artifact administration action %q", statement.Sid, action)
			}
		}
	}
}

func TestDevelopmentBootstrapPolicyAllowsOnlyReviewedReleaseRepositoryActions(t *testing.T) {
	_, policy := loadPolicy(t, "portfolio-deployer-development-bootstrap-policy.json")

	repositoryARN := "arn:aws:ecr:us-west-2:180294223248:repository/portfolio-lambda-releases"
	wantActions := stringSet(
		"ecr:BatchCheckLayerAvailability",
		"ecr:BatchGetImage",
		"ecr:CompleteLayerUpload",
		"ecr:DescribeImageScanFindings",
		"ecr:DescribeImages",
		"ecr:DescribeRepositories",
		"ecr:GetLifecyclePolicy",
		"ecr:GetRepositoryPolicy",
		"ecr:InitiateLayerUpload",
		"ecr:ListTagsForResource",
		"ecr:PutImage",
		"ecr:UploadLayerPart",
	)

	found := 0
	for _, statement := range policy.Statement {
		if !stringSetContains(stringSet(statement.Resource...), repositoryARN) {
			continue
		}

		found++
		if statement.Effect != "Allow" {
			t.Errorf("release repository statement effect = %q, want Allow", statement.Effect)
		}
		assertStringSet(t, "release repository resources", stringSet(statement.Resource...), stringSet(repositoryARN))
		assertStringSet(t, "release repository actions", stringSet(statement.Action...), wantActions)
	}

	if found != 1 {
		t.Fatalf("release repository statement count = %d, want 1", found)
	}
}

func TestDevelopmentBootstrapPolicyBindsRemainingAPIGatewayAccessToCapturedAPI(t *testing.T) {
	_, policy := loadPolicy(t, "portfolio-deployer-development-bootstrap-policy.json")

	want := map[string]struct {
		actions   map[string]struct{}
		resources map[string]struct{}
	}{
		"T8": {
			actions: stringSet("apigateway:GET", "apigateway:PATCH", "apigateway:POST"),
			resources: stringSet(
				"arn:aws:apigateway:us-west-2::/apis/048o6alxh8",
				"arn:aws:apigateway:us-west-2::/apis/048o6alxh8/*",
			),
		},
		"TK": {
			actions: stringSet("apigateway:PUT"),
			resources: stringSet(
				"arn:aws:apigateway:us-west-2::/tags/arn%3Aaws%3Aapigateway%3Aus-west-2%3A%3A%2Fapis%2F048o6alxh8%2Fstages%2F%24default",
			),
		},
		"TN": {
			actions: stringSet("apigateway:TagResource"),
			resources: stringSet(
				"arn:aws:apigateway:us-west-2::/apis/048o6alxh8/stages",
			),
		},
	}
	found := make(map[string]int, len(want))
	for _, statement := range policy.Statement {
		for _, action := range statement.Action {
			if !strings.HasPrefix(action, "apigateway:") {
				continue
			}
			if _, ok := want[statement.Sid]; !ok {
				t.Errorf("API Gateway action %q is owned by unreviewed statement %q", action, statement.Sid)
			}
		}

		expected, ok := want[statement.Sid]
		if !ok {
			continue
		}
		found[statement.Sid]++
		if statement.Effect != "Allow" {
			t.Errorf("API Gateway tag statement %q effect = %q, want Allow", statement.Sid, statement.Effect)
		}
		assertStringSet(t, "API Gateway tag actions for "+statement.Sid, stringSet(statement.Action...), expected.actions)
		assertStringSet(t, "API Gateway tag resources for "+statement.Sid, stringSet(statement.Resource...), expected.resources)
	}
	for sid := range want {
		if found[sid] != 1 {
			t.Errorf("API Gateway tag statement %q count = %d, want 1", sid, found[sid])
		}
	}
}

func TestDevelopmentBootstrapPolicyUsesExactCloudWatchLogARNForms(t *testing.T) {
	_, policy := loadPolicy(t, "portfolio-deployer-development-bootstrap-policy.json")

	want := map[string]struct {
		actions   map[string]struct{}
		resources map[string]struct{}
	}{
		"T9": {
			actions: stringSet("logs:TagResource"),
			resources: stringSet(
				"arn:aws:logs:us-west-2:180294223248:log-group:/aws/apigateway/portfolio-lambda-dev/access",
				"arn:aws:logs:us-west-2:180294223248:log-group:/aws/apigateway/portfolio-lambda-dev/access:*",
				"arn:aws:logs:us-west-2:180294223248:log-group:/aws/lambda/portfolio-lambda-dev",
				"arn:aws:logs:us-west-2:180294223248:log-group:/aws/lambda/portfolio-lambda-dev:*",
			),
		},
		"TM": {
			actions: stringSet("logs:CreateLogGroup"),
			resources: stringSet(
				"arn:aws:logs:us-west-2:180294223248:log-group:/aws/apigateway/portfolio-lambda-dev/access:*",
				"arn:aws:logs:us-west-2:180294223248:log-group:/aws/lambda/portfolio-lambda-dev:*",
			),
		},
	}
	found := make(map[string]int, len(want))
	for _, statement := range policy.Statement {
		for _, action := range statement.Action {
			switch action {
			case "logs:TagResource":
				if statement.Sid != "T9" {
					t.Errorf("CloudWatch Logs action %q is owned by statement %q, want T9", action, statement.Sid)
				}
			case "logs:CreateLogGroup":
				if statement.Sid != "TM" {
					t.Errorf("CloudWatch Logs action %q is owned by statement %q, want TM", action, statement.Sid)
				}
			case "logs:TagLogGroup":
				t.Errorf("statement %q retains deprecated CloudWatch Logs action %q", statement.Sid, action)
			}
		}

		expected, ok := want[statement.Sid]
		if !ok {
			continue
		}
		found[statement.Sid]++
		if statement.Effect != "Allow" {
			t.Errorf("CloudWatch Logs statement %q effect = %q, want Allow", statement.Sid, statement.Effect)
		}
		assertStringSet(t, "CloudWatch Logs actions for "+statement.Sid, stringSet(statement.Action...), expected.actions)
		assertStringSet(t, "CloudWatch Logs resources for "+statement.Sid, stringSet(statement.Resource...), expected.resources)
	}
	for sid := range want {
		if found[sid] != 1 {
			t.Errorf("CloudWatch Logs statement %q count = %d, want 1", sid, found[sid])
		}
	}
}

func TestDevelopmentBootstrapPolicyAllowsOnlyParameterCreatesWithoutOverwriteOrPolicies(t *testing.T) {
	_, policy := loadPolicy(t, "portfolio-deployer-development-bootstrap-policy.json")

	wantResources := stringSet(
		"arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/dev/CLIENT_ID_KEY",
		"arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/dev/CLIENT_SECRET_KEY",
		"arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/dev/LPS_SESSION_KEY",
	)
	allowCount := 0
	overwriteDenyCount := 0
	policiesDenyCount := 0
	for i := range policy.Statement {
		statement := &policy.Statement[i]
		if !stringSetContains(stringSet(statement.Action...), "ssm:PutParameter") {
			continue
		}
		assertStringSet(t, "PutParameter actions", stringSet(statement.Action...), stringSet("ssm:PutParameter"))
		assertStringSet(t, "PutParameter resources", stringSet(statement.Resource...), wantResources)
		switch statement.Effect {
		case "Allow":
			allowCount++
			if len(statement.Condition) != 0 {
				t.Errorf("PutParameter allow condition = %#v, want unconditional exact-resource allow", statement.Condition)
			}
		case "Deny":
			if len(statement.Condition) != 1 {
				t.Errorf("PutParameter deny condition = %#v, want one StringEquals condition", statement.Condition)
				continue
			}
			denyEquals, ok := statement.Condition["StringEquals"].(map[string]any)
			if !ok || len(denyEquals) != 1 {
				t.Errorf("PutParameter deny condition = %#v, want one exact StringEquals key", statement.Condition)
				continue
			}
			switch {
			case reflect.DeepEqual(denyEquals, map[string]any{"ssm:Overwrite": "true"}):
				overwriteDenyCount++
			case reflect.DeepEqual(denyEquals, map[string]any{"ssm:Policies": "true"}):
				policiesDenyCount++
			default:
				t.Errorf("PutParameter deny condition = %#v, want overwrite=true or policies=true", statement.Condition)
			}
		default:
			t.Errorf("PutParameter statement effect = %q, want Allow or Deny", statement.Effect)
		}
	}

	if allowCount != 1 || overwriteDenyCount != 1 || policiesDenyCount != 1 {
		t.Fatalf(
			"PutParameter statement counts = allow %d, overwrite deny %d, policies deny %d; want exactly one each",
			allowCount,
			overwriteDenyCount,
			policiesDenyCount,
		)
	}
}

func TestExecutionBoundarySeparatesDevelopmentAndProductionRoles(t *testing.T) {
	_, policy := loadPolicy(t, "portfolio-lambda-execution-boundary-policy.json")

	type environmentPair struct {
		dev  *policyStatement
		prod *policyStatement
	}
	pairs := make(map[string]*environmentPair)
	for i := range policy.Statement {
		statement := &policy.Statement[i]
		environment, suffix := statementEnvironment(t, statement.Sid)
		pair := pairs[suffix]
		if pair == nil {
			pair = &environmentPair{}
			pairs[suffix] = pair
		}
		if environment == "dev" {
			pair.dev = statement
		} else {
			pair.prod = statement
		}

		wantRole := "arn:aws:iam::180294223248:role/portfolio-lambda-" + environment + "-execution"
		if got := principalARN(t, statement); got != wantRole {
			t.Errorf("statement %q principal ARN = %q, want %q", statement.Sid, got, wantRole)
		}
		encoded, err := json.Marshal(statement)
		if err != nil {
			t.Fatalf("marshal boundary statement %q: %v", statement.Sid, err)
		}
		otherEnvironment := "prod"
		if environment == "prod" {
			otherEnvironment = "dev"
		}
		if strings.Contains(string(encoded), "-"+otherEnvironment+"-") || strings.Contains(string(encoded), "/"+otherEnvironment+"/") {
			t.Errorf("statement %q crosses into %s resources or principal", statement.Sid, otherEnvironment)
		}

		for _, action := range statement.Action {
			service, _, ok := strings.Cut(action, ":")
			if !ok || !stringSetContains(stringSet("dynamodb", "kms", "logs", "ssm"), service) {
				t.Errorf("statement %q has non-runtime action %q", statement.Sid, action)
			}
			if action == "logs:CreateLogGroup" || action == "*" || strings.HasSuffix(action, ":*") {
				t.Errorf("statement %q exceeds the runtime ceiling with %q", statement.Sid, action)
			}
		}
		for _, resource := range statement.Resource {
			if resource == "*" {
				t.Errorf("statement %q has an unscoped resource", statement.Sid)
			}
		}
	}

	expectedPairs := stringSet(
		"GoogleConnections",
		"LambdaLogs",
		"ParameterDecryption",
		"Parameters",
		"SoccerSessions",
	)
	if got := stringSetFromKeys(pairs); !reflect.DeepEqual(got, expectedPairs) {
		t.Errorf("boundary statement pairs = %v, want %v", sortedSet(got), sortedSet(expectedPairs))
	}
	for suffix, pair := range pairs {
		if pair.dev == nil || pair.prod == nil {
			t.Errorf("boundary pair %q is incomplete", suffix)
			continue
		}
		if !reflect.DeepEqual(sortedStrings(pair.dev.Action), sortedStrings(pair.prod.Action)) {
			t.Errorf("boundary pair %q has asymmetric actions: dev=%v prod=%v", suffix, pair.dev.Action, pair.prod.Action)
		}
		if !reflect.DeepEqual(normalizedResources(pair.dev.Resource, "dev"), normalizedResources(pair.prod.Resource, "prod")) {
			t.Errorf("boundary pair %q has asymmetric resources: dev=%v prod=%v", suffix, pair.dev.Resource, pair.prod.Resource)
		}
	}
}

func loadPolicy(t *testing.T, path string) ([]byte, policyDocument) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read policy %q: %v", path, err)
	}
	var policy policyDocument
	if err := json.Unmarshal(data, &policy); err != nil {
		t.Fatalf("parse policy %q: %v", path, err)
	}
	return data, policy
}

func nonWhitespaceByteCount(data []byte) int {
	count := 0
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if !unicode.IsSpace(r) {
			count += size
		}
		data = data[size:]
	}
	return count
}

func containsLegacySource(statement string) bool {
	return strings.Contains(statement, "portfolio/terraform.tfstate") ||
		strings.Contains(statement, "parameter/portfolio/CLIENT_ID_KEY") ||
		strings.Contains(statement, "parameter/portfolio/CLIENT_SECRET_KEY")
}

func statementEnvironment(t *testing.T, sid string) (string, string) {
	t.Helper()
	if strings.HasPrefix(sid, "Dev") {
		return "dev", strings.TrimPrefix(sid, "Dev")
	}
	if strings.HasPrefix(sid, "Prod") {
		return "prod", strings.TrimPrefix(sid, "Prod")
	}
	t.Fatalf("boundary statement Sid %q does not identify an environment", sid)
	return "", ""
}

func principalARN(t *testing.T, statement *policyStatement) string {
	t.Helper()
	arnEquals, ok := statement.Condition["ArnEquals"].(map[string]any)
	if !ok {
		t.Fatalf("statement %q lacks ArnEquals conditions", statement.Sid)
	}
	principal, ok := arnEquals["aws:PrincipalArn"].(string)
	if !ok {
		t.Fatalf("statement %q lacks one string aws:PrincipalArn", statement.Sid)
	}
	return principal
}

func normalizedResources(resources []string, environment string) []string {
	normalized := make([]string, 0, len(resources))
	for _, resource := range resources {
		resource = strings.ReplaceAll(resource, "-"+environment+"-", "-{environment}-")
		resource = strings.ReplaceAll(resource, "-"+environment+":", "-{environment}:")
		resource = strings.ReplaceAll(resource, "/"+environment+"/", "/{environment}/")
		normalized = append(normalized, resource)
	}
	return sortedStrings(normalized)
}

func stringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func stringSetContains(set map[string]struct{}, value string) bool {
	_, ok := set[value]
	return ok
}

func stringSetFromKeys[V any](values map[string]V) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for value := range values {
		set[value] = struct{}{}
	}
	return set
}

func assertStringSet(t *testing.T, label string, got, want map[string]struct{}) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %v, want %v", label, sortedSet(got), sortedSet(want))
	}
}

func sortedSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	return sortedStrings(result)
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
