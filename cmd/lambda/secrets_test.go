package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type fakeSSMGetter struct {
	output *ssm.GetParametersOutput
	err    error
}

func (fake *fakeSSMGetter) GetParameters(
	_ context.Context,
	_ *ssm.GetParametersInput,
	_ ...func(*ssm.Options),
) (*ssm.GetParametersOutput, error) {
	return fake.output, fake.err
}

func ssmParameter(name, value string) types.Parameter {
	lastModified := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	return types.Parameter{
		ARN:              aws.String("arn:aws:ssm:us-east-1:123456789012:parameter" + name),
		DataType:         aws.String("text"),
		LastModifiedDate: &lastModified,
		Name:             aws.String(name),
		Selector:         aws.String(name),
		SourceResult:     aws.String("{}"),
		Type:             types.ParameterTypeSecureString,
		Value:            aws.String(value),
		Version:          1,
	}
}

func setSSMPathEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CLIENT_ID_KEY", "/portfolio/client-id")
	t.Setenv("CLIENT_SECRET_KEY", "/portfolio/client-secret")
	t.Setenv("LPS_SESSION_KEY", "/portfolio/lps-session")
}

func assertSSMEnv(t *testing.T, wantClientID, wantClientSecret, wantLPSSession string) {
	t.Helper()
	for name, want := range map[string]string{
		"CLIENT_ID_KEY":     wantClientID,
		"CLIENT_SECRET_KEY": wantClientSecret,
		"LPS_SESSION_KEY":   wantLPSSession,
	} {
		if got := os.Getenv(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// Production break caught: successful SSM responses that are never applied
// leave Lambda configured with parameter paths instead of usable credentials.
func TestResolveSSMCompleteResponseUpdatesAllParameterPaths(t *testing.T) {
	setSSMPathEnv(t)
	client := &fakeSSMGetter{output: &ssm.GetParametersOutput{Parameters: []types.Parameter{
		ssmParameter("/portfolio/client-id", "resolved-client-id"),
		ssmParameter("/portfolio/client-secret", "resolved-client-secret"),
		ssmParameter("/portfolio/lps-session", "resolved-lps-session"),
	}}}

	if err := resolveSSMSecretsWithClient(t.Context(), client); err != nil {
		t.Fatalf("resolve SSM secrets: %v", err)
	}
	assertSSMEnv(t, "resolved-client-id", "resolved-client-secret", "resolved-lps-session")
}

// Production break caught: dereferencing or accepting a missing SSM response
// can panic or replace only a subset of the Lambda configuration.
func TestResolveSSMMissingResponseLeavesEnvironmentUnchanged(t *testing.T) {
	setSSMPathEnv(t)
	client := &fakeSSMGetter{}

	if err := resolveSSMSecretsWithClient(t.Context(), client); err == nil {
		t.Fatal("resolve SSM secrets unexpectedly succeeded")
	}
	assertSSMEnv(t, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session")
}

// Production break caught: attempting to resolve a literal setting corrupts a
// configured value that is already available without SSM.
func TestResolveSSMLiteralValueRemainsUnchanged(t *testing.T) {
	t.Setenv("CLIENT_ID_KEY", "literal-client-id")
	t.Setenv("CLIENT_SECRET_KEY", "/portfolio/client-secret")
	t.Setenv("LPS_SESSION_KEY", "/portfolio/lps-session")
	client := &fakeSSMGetter{output: &ssm.GetParametersOutput{Parameters: []types.Parameter{
		ssmParameter("/portfolio/client-secret", "resolved-client-secret"),
		ssmParameter("/portfolio/lps-session", "resolved-lps-session"),
	}}}

	if err := resolveSSMSecretsWithClient(t.Context(), client); err != nil {
		t.Fatalf("resolve SSM secrets: %v", err)
	}
	assertSSMEnv(t, "literal-client-id", "resolved-client-secret", "resolved-lps-session")
}

// Production break caught: applying parameters while walking a partial response
// leaks a mixed path/plaintext configuration into Lambda initialization.
func TestResolveSSMPartialResponseLeavesEnvironmentUnchanged(t *testing.T) {
	setSSMPathEnv(t)
	client := &fakeSSMGetter{output: &ssm.GetParametersOutput{Parameters: []types.Parameter{
		ssmParameter("/portfolio/client-id", "resolved-client-id"),
		ssmParameter("/portfolio/lps-session", "resolved-lps-session"),
	}}}

	if err := resolveSSMSecretsWithClient(t.Context(), client); err == nil {
		t.Fatal("resolve SSM secrets unexpectedly succeeded")
	}
	assertSSMEnv(t, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session")
}

// Production break caught: ignoring InvalidParameters permits a partially
// resolved configuration even though SSM explicitly rejected a requested path.
func TestResolveSSMInvalidParametersLeaveEnvironmentUnchanged(t *testing.T) {
	setSSMPathEnv(t)
	client := &fakeSSMGetter{output: &ssm.GetParametersOutput{
		Parameters: []types.Parameter{
			ssmParameter("/portfolio/client-id", "resolved-client-id"),
			ssmParameter("/portfolio/lps-session", "resolved-lps-session"),
		},
		InvalidParameters: []string{"/portfolio/client-secret"},
	}}

	if err := resolveSSMSecretsWithClient(t.Context(), client); err == nil {
		t.Fatal("resolve SSM secrets unexpectedly succeeded")
	}
	assertSSMEnv(t, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session")
}

// Production break caught: updating from an output accompanying a client error
// replaces configuration despite an unsuccessful SSM operation.
func TestResolveSSMClientErrorLeavesEnvironmentUnchanged(t *testing.T) {
	setSSMPathEnv(t)
	client := &fakeSSMGetter{
		output: &ssm.GetParametersOutput{Parameters: []types.Parameter{
			ssmParameter("/portfolio/client-id", "resolved-client-id"),
			ssmParameter("/portfolio/client-secret", "resolved-client-secret"),
			ssmParameter("/portfolio/lps-session", "resolved-lps-session"),
		}},
		err: errors.New("SSM unavailable"),
	}

	if err := resolveSSMSecretsWithClient(t.Context(), client); err == nil {
		t.Fatal("resolve SSM secrets unexpectedly succeeded")
	}
	assertSSMEnv(t, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session")
}
