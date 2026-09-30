package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"portfolio/internal/config"
)

type fakeSSMGetter struct {
	output    *ssm.GetParametersOutput
	err       error
	calls     int
	inputs    []*ssm.GetParametersInput
	responses []fakeSSMResponse
}

type fakeSSMResponse struct {
	output *ssm.GetParametersOutput
	err    error
}

func (fake *fakeSSMGetter) GetParameters(
	_ context.Context,
	input *ssm.GetParametersInput,
	_ ...func(*ssm.Options),
) (*ssm.GetParametersOutput, error) {
	fake.calls++
	fake.inputs = append(fake.inputs, input)
	if len(fake.responses) >= fake.calls {
		response := fake.responses[fake.calls-1]
		return response.output, response.err
	}
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
	t.Setenv("MGMT_SESSION_KEY", "")
	t.Setenv("SITE_SESSION_KEY", "")
}

func TestResolveSSMSiteSessionKeyUsesItsEnvironmentParameter(t *testing.T) {
	setSSMPathEnv(t)
	path := "/portfolio/lambda/prod/SITE_SESSION_KEY"
	t.Setenv("SITE_SESSION_KEY", path)
	client := &fakeSSMGetter{responses: []fakeSSMResponse{
		{output: &ssm.GetParametersOutput{Parameters: []types.Parameter{
			ssmParameter("/portfolio/client-id", "resolved-client-id"),
			ssmParameter("/portfolio/client-secret", "resolved-client-secret"),
			ssmParameter("/portfolio/lps-session", "resolved-lps-session"),
		}}},
		{output: &ssm.GetParametersOutput{Parameters: []types.Parameter{
			ssmParameter(path, strings.Repeat("ab", 32)),
		}}},
	}}
	if err := resolveSSMSecretsWithClient(t.Context(), client); err != nil {
		t.Fatalf("resolve SSM secrets: %v", err)
	}
	assertSSMRequests(t, client,
		[]string{"/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session"},
		[]string{path},
	)
	if got := os.Getenv("SITE_SESSION_KEY"); got != strings.Repeat("ab", 32) {
		t.Fatalf("SITE_SESSION_KEY = %q, want decrypted key", got)
	}
}

func TestResolveSSMMissingSiteSessionKeyLeavesPublicRuntimeAvailable(t *testing.T) {
	setSSMPathEnv(t)
	path := "/portfolio/lambda/prod/SITE_SESSION_KEY"
	t.Setenv("SITE_SESSION_KEY", path)
	client := &fakeSSMGetter{responses: []fakeSSMResponse{
		{output: &ssm.GetParametersOutput{Parameters: []types.Parameter{
			ssmParameter("/portfolio/client-id", "resolved-client-id"),
			ssmParameter("/portfolio/client-secret", "resolved-client-secret"),
			ssmParameter("/portfolio/lps-session", "resolved-lps-session"),
		}}},
		{output: &ssm.GetParametersOutput{InvalidParameters: []string{path}}},
	}}
	if err := resolveSSMSecretsWithClient(t.Context(), client); err != nil {
		t.Fatalf("optional site key failure stopped public runtime: %v", err)
	}
	if _, ok := os.LookupEnv("SITE_SESSION_KEY"); ok {
		t.Fatal("failed site key left an SSM path available to site configuration")
	}
	assertSSMEnv(t, "resolved-client-id", "resolved-client-secret", "resolved-lps-session", "")
}

// A production runtime whose only SSM path is the optional site key must still
// start when AWS configuration cannot load, with site sign-in off rather than
// configured from a parameter path.
func TestResolveSSMAWSConfigFailureLeavesSiteSignInOffAndPublicRuntimeUp(t *testing.T) {
	t.Setenv("AWS_MAX_ATTEMPTS", "invalid")
	t.Setenv("CLIENT_ID_KEY", "")
	t.Setenv("CLIENT_SECRET_KEY", "")
	t.Setenv("LPS_SESSION_KEY", "")
	t.Setenv("MGMT_SESSION_KEY", "")
	t.Setenv("SITE_SESSION_KEY", "/portfolio/lambda/prod/SITE_SESSION_KEY")

	if err := resolveSSMSecrets(t.Context()); err != nil {
		t.Fatalf("optional site key AWS config failure prevented startup: %v", err)
	}
	if value, ok := os.LookupEnv("SITE_SESSION_KEY"); ok {
		t.Fatalf("SITE_SESSION_KEY = %q after AWS config failure, want unset", value)
	}
}

func assertSSMEnv(t *testing.T, wantClientID, wantClientSecret, wantLPSSession, wantManagement string) {
	t.Helper()
	for name, want := range map[string]string{
		"CLIENT_ID_KEY":     wantClientID,
		"CLIENT_SECRET_KEY": wantClientSecret,
		"LPS_SESSION_KEY":   wantLPSSession,
		"MGMT_SESSION_KEY":  wantManagement,
	} {
		if got := os.Getenv(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func assertSSMRequest(t *testing.T, client *fakeSSMGetter, wantPaths ...string) {
	t.Helper()
	assertSSMRequests(t, client, wantPaths)
}

func assertSSMRequests(t *testing.T, client *fakeSSMGetter, wantRequests ...[]string) {
	t.Helper()
	if client.calls != len(wantRequests) {
		t.Fatalf("GetParameters calls = %d, want %d", client.calls, len(wantRequests))
	}
	for i, wantPaths := range wantRequests {
		input := client.inputs[i]
		if !slices.Equal(input.Names, wantPaths) {
			t.Fatalf("GetParameters request %d names = %q, want %q", i, input.Names, wantPaths)
		}
		if input.WithDecryption == nil || !*input.WithDecryption {
			t.Fatal("GetParameters WithDecryption = false, want true")
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
	assertSSMRequest(t, client, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session")
	assertSSMEnv(t, "resolved-client-id", "resolved-client-secret", "resolved-lps-session", "")
}

// Production break caught: fetching the retired management session key kept a
// secret read alive only to feed a setting the application ignores. A path the
// environment still carries stays in place, so config names it in its
// retirement warning without logging the path.
func TestResolveSSMLeavesRetiredManagementSessionPathForConfigWarning(t *testing.T) {
	path := "/portfolio/lambda/dev/MGMT_SESSION_KEY"
	setSSMPathEnv(t)
	t.Setenv("MGMT_SESSION_KEY", path)
	client := &fakeSSMGetter{output: &ssm.GetParametersOutput{Parameters: []types.Parameter{
		ssmParameter("/portfolio/client-id", "resolved-client-id"),
		ssmParameter("/portfolio/client-secret", "resolved-client-secret"),
		ssmParameter("/portfolio/lps-session", "resolved-lps-session"),
	}}}

	if err := resolveSSMSecretsWithClient(t.Context(), client); err != nil {
		t.Fatalf("resolve SSM secrets: %v", err)
	}
	assertSSMRequest(t, client, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session")
	assertSSMEnv(t, "resolved-client-id", "resolved-client-secret", "resolved-lps-session", path)

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	config.Load()
	if !strings.Contains(logs.String(), "retired management-only identity settings are ignored") || !strings.Contains(logs.String(), "MGMT_SESSION_KEY") {
		t.Fatal("config did not warn that the management session key is retired")
	}
	if strings.Contains(logs.String(), path) {
		t.Fatal("retirement warning logged the parameter path")
	}
}

func TestResolveSSMRetiredManagementSessionPathAloneNeedsNoSSM(t *testing.T) {
	t.Setenv("CLIENT_ID_KEY", "literal-client-id")
	t.Setenv("CLIENT_SECRET_KEY", "literal-client-secret")
	t.Setenv("LPS_SESSION_KEY", "literal-lps-session")
	t.Setenv("MGMT_SESSION_KEY", "/portfolio/lambda/dev/MGMT_SESSION_KEY")
	client := &fakeSSMGetter{err: errors.New("SSM must not be called")}

	if err := resolveSSMSecretsWithClient(t.Context(), client); err != nil {
		t.Fatalf("resolve SSM secrets: %v", err)
	}
	if client.calls != 0 {
		t.Fatalf("GetParameters calls = %d, want 0", client.calls)
	}
	assertSSMEnv(t, "literal-client-id", "literal-client-secret", "literal-lps-session", "/portfolio/lambda/dev/MGMT_SESSION_KEY")
}

// Production break caught: dereferencing or accepting a missing SSM response
// can panic or replace only a subset of the Lambda configuration.
func TestResolveSSMMissingResponseLeavesEnvironmentUnchanged(t *testing.T) {
	setSSMPathEnv(t)
	client := &fakeSSMGetter{}

	if err := resolveSSMSecretsWithClient(t.Context(), client); err == nil {
		t.Fatal("resolve SSM secrets unexpectedly succeeded")
	}
	assertSSMRequest(t, client, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session")
	assertSSMEnv(t, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session", "")
}

// Production break caught: attempting to resolve a literal setting corrupts a
// configured value that is already available without SSM.
func TestResolveSSMLiteralValueRemainsUnchanged(t *testing.T) {
	t.Setenv("CLIENT_ID_KEY", "literal-client-id")
	t.Setenv("CLIENT_SECRET_KEY", "/portfolio/client-secret")
	t.Setenv("LPS_SESSION_KEY", "/portfolio/lps-session")
	t.Setenv("MGMT_SESSION_KEY", "literal-management-session")
	t.Setenv("MGMT_COGNITO_DOMAIN", "/portfolio/lambda/dev/MGMT_COGNITO_DOMAIN")
	client := &fakeSSMGetter{output: &ssm.GetParametersOutput{Parameters: []types.Parameter{
		ssmParameter("/portfolio/client-secret", "resolved-client-secret"),
		ssmParameter("/portfolio/lps-session", "resolved-lps-session"),
	}}}

	if err := resolveSSMSecretsWithClient(t.Context(), client); err != nil {
		t.Fatalf("resolve SSM secrets: %v", err)
	}
	assertSSMRequest(t, client, "/portfolio/client-secret", "/portfolio/lps-session")
	assertSSMEnv(t, "literal-client-id", "resolved-client-secret", "resolved-lps-session", "literal-management-session")
	if got := os.Getenv("MGMT_COGNITO_DOMAIN"); got != "/portfolio/lambda/dev/MGMT_COGNITO_DOMAIN" {
		t.Fatalf("MGMT_COGNITO_DOMAIN = %q, want literal path unchanged", got)
	}
}

func TestResolveSSMAWSConfigFailureStopsStartupOnlyForRequiredPaths(t *testing.T) {
	t.Setenv("AWS_MAX_ATTEMPTS", "invalid")
	for _, required := range []bool{false, true} {
		name := "retired management path only"
		if required {
			name = "required secrets"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("CLIENT_ID_KEY", "")
			t.Setenv("CLIENT_SECRET_KEY", "")
			t.Setenv("LPS_SESSION_KEY", "")
			t.Setenv("MGMT_SESSION_KEY", "/portfolio/lambda/dev/MGMT_SESSION_KEY")
			if required {
				t.Setenv("CLIENT_ID_KEY", "/portfolio/client-id")
			}

			err := resolveSSMSecrets(t.Context())
			if required {
				if err == nil || !strings.Contains(err.Error(), "load AWS config") {
					t.Fatalf("required AWS config failure = %v, want startup error", err)
				}
				assertSSMEnv(t, "/portfolio/client-id", "", "", "/portfolio/lambda/dev/MGMT_SESSION_KEY")
			} else {
				if err != nil {
					t.Fatalf("retired management path loaded AWS config: %v", err)
				}
				assertSSMEnv(t, "", "", "", "/portfolio/lambda/dev/MGMT_SESSION_KEY")
			}
		})
	}
}

// Production break caught: applying parameters while walking a partial response
// leaks a mixed path/plaintext configuration into Lambda initialization.
func TestResolveSSMPartialResponseLeavesEnvironmentUnchanged(t *testing.T) {
	setSSMPathEnv(t)
	t.Setenv("MGMT_SESSION_KEY", "/portfolio/lambda/dev/MGMT_SESSION_KEY")
	client := &fakeSSMGetter{output: &ssm.GetParametersOutput{Parameters: []types.Parameter{
		ssmParameter("/portfolio/client-id", "resolved-client-id"),
		ssmParameter("/portfolio/lps-session", "resolved-lps-session"),
	}}}

	if err := resolveSSMSecretsWithClient(t.Context(), client); err == nil {
		t.Fatal("resolve SSM secrets unexpectedly succeeded")
	}
	assertSSMRequest(t, client, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session")
	assertSSMEnv(t, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session", "/portfolio/lambda/dev/MGMT_SESSION_KEY")
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
	assertSSMRequest(t, client, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session")
	assertSSMEnv(t, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session", "")
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
	assertSSMRequest(t, client, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session")
	assertSSMEnv(t, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session", "")
}

// Production break caught: a later value rejected by os.Setenv can leave an
// earlier key resolved, producing a mixed Lambda configuration after an error.
func TestResolveSSMInvalidResolvedValueLeavesEnvironmentUnchanged(t *testing.T) {
	setSSMPathEnv(t)
	client := &fakeSSMGetter{output: &ssm.GetParametersOutput{Parameters: []types.Parameter{
		ssmParameter("/portfolio/client-id", "resolved-client-id"),
		ssmParameter("/portfolio/client-secret", "invalid\x00client-secret"),
		ssmParameter("/portfolio/lps-session", "resolved-lps-session"),
	}}}

	if err := resolveSSMSecretsWithClient(t.Context(), client); err == nil {
		t.Fatal("resolve SSM secrets unexpectedly succeeded")
	}
	assertSSMRequest(t, client, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session")
	assertSSMEnv(t, "/portfolio/client-id", "/portfolio/client-secret", "/portfolio/lps-session", "")
}
