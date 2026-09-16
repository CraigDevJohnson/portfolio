package bootstrap

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestManagementCandidatesPreserveApprovedPolicyScope(t *testing.T) {
	read := func(path string) policyDocument {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var doc policyDocument
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	original := read("portfolio-lambda-execution-boundary-policy.json")
	candidate := read("candidates/portfolio-lambda-execution-boundary-management-candidate.json")
	if len(candidate.Statement) != len(original.Statement)+3 {
		t.Fatal("unexpected boundary topology")
	}
	parameter := "arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/dev/MGMT_SESSION_KEY"
	for i, want := range original.Statement {
		got := candidate.Statement[i]
		switch want.Sid {
		case "DevParameters":
			want.Resource = append(want.Resource, parameter)
		case "DevParameterDecryption":
			condition := want.Condition["StringEquals"].(map[string]any)
			condition["kms:EncryptionContext:PARAMETER_ARN"] = append(condition["kms:EncryptionContext:PARAMETER_ARN"].([]any), parameter)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("baseline statement %s changed beyond its exact session grant", want.Sid)
		}
	}
	wantedActions := []stringList{{"ec2:DescribeInstances", "cloudwatch:GetMetricStatistics"}, {"logs:FilterLogEvents"}, {"ec2:StartInstances", "ec2:StopInstances"}}
	wantedResources := []stringList{{"*"}, {"arn:aws:logs:us-west-2:180294223248:log-group:/ec2/i-*:*"}, {"arn:aws:ec2:us-west-2:180294223248:instance/*"}}
	for i, st := range candidate.Statement[len(original.Statement):] {
		if st.Effect != "Allow" || !reflect.DeepEqual(st.Action, wantedActions[i]) || !reflect.DeepEqual(st.Resource, wantedResources[i]) {
			t.Errorf("management scope drift: %s", st.Sid)
		}
		condition := map[string]any{"ArnEquals": map[string]any{"aws:PrincipalArn": "arn:aws:iam::180294223248:role/portfolio-lambda-dev-execution"}}
		if i == 0 {
			condition["StringEquals"] = map[string]any{"aws:RequestedRegion": "us-west-2"}
		}
		if i == 2 {
			condition["StringEquals"] = map[string]any{"ec2:ResourceTag/PortfolioManagement": "dev"}
		}
		if !reflect.DeepEqual(st.Condition, condition) {
			t.Errorf("management boundary condition drift: %s", st.Sid)
		}
	}
	deployer := read("portfolio-deployer-development-bootstrap-policy.json")
	enhanced := read("candidates/portfolio-deployer-development-management-candidate.json")
	if len(enhanced.Statement) != len(deployer.Statement)+2 || !reflect.DeepEqual(enhanced.Statement[:len(deployer.Statement)], deployer.Statement) {
		t.Fatal("deployer candidate must preserve its full approved baseline")
	}
	setup := read("candidates/portfolio-deployer-auth-development-setup-candidate.json")
	for _, st := range setup.Statement {
		for _, action := range st.Action {
			if strings.HasPrefix(action, "cognito-idp:Delete") {
				t.Fatal("provisioning setup must not authorize Cognito deletion")
			}
			if !strings.HasPrefix(action, "s3:") && !strings.HasPrefix(action, "cognito-idp:") {
				t.Fatalf("auth setup adds unrelated permission: %s", action)
			}
		}
		for _, resource := range st.Resource {
			if strings.Contains(resource, "/prod/") || strings.Contains(resource, ":iam:") {
				t.Fatalf("auth setup broadens production or IAM: %s", resource)
			}
		}
	}
	if len(setup.Statement) != 8 {
		t.Fatal("unexpected auth setup topology")
	}
	for _, st := range setup.Statement {
		if st.Sid == "AuthStateObject" && (!reflect.DeepEqual(st.Action, stringList{"s3:GetObject", "s3:PutObject"}) || !reflect.DeepEqual(st.Resource, stringList{"arn:aws:s3:::portfolio-tofu-state-180294223248/portfolio-lambda-http-api/auth/dev/terraform.tfstate"})) {
			t.Fatal("auth state access must remain exact and cannot delete state")
		}
	}
}
