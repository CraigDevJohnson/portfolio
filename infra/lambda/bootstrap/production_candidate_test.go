package bootstrap

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestProductionBootstrapCandidateIsTemporaryAndAdditive(t *testing.T) {
	data, err := os.ReadFile("candidates/portfolio-production-bootstrap-candidate.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc policyDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != "2012-10-17" || len(doc.Statement) != 14 {
		t.Fatal("unexpected bootstrap policy shape")
	}
	if nonWhitespaceByteCount(data) > 6144 {
		t.Fatal("candidate exceeds customer managed policy size limit")
	}
	bySID := make(map[string]policyStatement)
	for _, st := range doc.Statement {
		if st.Effect != "Allow" || st.Sid == "" {
			t.Fatalf("unexpected effect or missing SID: %s", st.Sid)
		}
		if _, exists := bySID[st.Sid]; exists {
			t.Fatalf("duplicate SID: %s", st.Sid)
		}
		bySID[st.Sid] = st
		want := map[string]any{"aws:CurrentTime": "2026-09-28T06:43:37Z"}
		if !reflect.DeepEqual(st.Condition["DateLessThan"], want) {
			t.Fatalf("grant does not expire at reviewed deadline: %s", st.Sid)
		}
		for _, action := range st.Action {
			if strings.Contains(action, "*") || strings.HasPrefix(action, "ecr:") ||
				strings.HasPrefix(action, "acm:") || strings.HasPrefix(action, "kms:") ||
				strings.Contains(action, "Delete") && !(st.Sid == "ProdLock" && action == "s3:DeleteObject") {
				t.Fatalf("unexpected broad, domain, credential or destructive grant: %s", action)
			}
			if strings.HasPrefix(action, "ssm:") && action != "ssm:DescribeParameters" {
				t.Fatalf("parameter values or mutations are not metadata: %s", action)
			}
		}
		for _, resource := range st.Resource {
			if strings.Contains(resource, "dev") || strings.Contains(resource, "domainnames") ||
				strings.Contains(resource, "artifacts/") || strings.Contains(resource, "ci-roles/") {
				t.Fatalf("unrelated resource scope: %s", resource)
			}
			if resource == "*" && st.Sid != "ParameterMetadata" {
				t.Fatalf("unscoped resource: %s", st.Sid)
			}
		}
	}
	boundary := "arn:aws:iam::180294223248:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary"
	role := stringList{"arn:aws:iam::180294223248:role/portfolio-lambda-prod-execution"}
	if !reflect.DeepEqual(bySID["ProdRoleSetup"].Resource, role) ||
		!reflect.DeepEqual(bySID["ProdRoleSetup"].Action, stringList{"iam:CreateRole", "iam:PutRolePolicy"}) ||
		!reflect.DeepEqual(bySID["ProdRoleSetup"].Condition["StringEquals"], map[string]any{"iam:PermissionsBoundary": boundary}) {
		t.Fatal("role setup must retain its exact boundary")
	}
	if !reflect.DeepEqual(bySID["ProdPassRole"].Resource, role) ||
		!reflect.DeepEqual(bySID["ProdPassRole"].Condition["StringEquals"], map[string]any{"iam:PassedToService": "lambda.amazonaws.com"}) {
		t.Fatal("pass-role broadened")
	}
	state := "arn:aws:s3:::portfolio-tofu-state-180294223248/portfolio-lambda-http-api/prod/terraform.tfstate"
	if !reflect.DeepEqual(bySID["ProdState"].Resource, stringList{state}) ||
		!reflect.DeepEqual(bySID["ProdState"].Action, stringList{"s3:GetObject", "s3:PutObject"}) ||
		!reflect.DeepEqual(bySID["ProdLock"].Resource, stringList{state + ".tflock"}) {
		t.Fatal("state ownership or deletion boundary changed")
	}
	if !reflect.DeepEqual(bySID["ProdApiCreate"].Resource, stringList{"arn:aws:apigateway:us-west-2::/apis"}) ||
		!reflect.DeepEqual(bySID["ProdApiCreate"].Condition["StringEquals"], map[string]any{
			"aws:RequestedRegion": "us-west-2", "apigateway:Request/ApiName": "portfolio-lambda-prod-http",
		}) {
		t.Fatal("API creation is not bound to the production name")
	}
	if !reflect.DeepEqual(bySID["ProdApiRead"].Action, stringList{"apigateway:GET"}) ||
		!reflect.DeepEqual(bySID["ProdApiInvoke"].Condition["StringEquals"], map[string]any{"lambda:Principal": "apigateway.amazonaws.com"}) {
		t.Fatal("API read or invocation scope changed")
	}
}
