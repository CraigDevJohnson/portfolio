package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// ssmSecretEnvVars lists the environment variable names whose values are SSM
// parameter paths that must be resolved to their plaintext secrets before the
// application configuration is loaded. The retired MGMT_SESSION_KEY is not
// resolved: a path left in it only makes config log its retirement warning.
var ssmSecretEnvVars = []string{"CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "LPS_SESSION_KEY"}

const siteSessionKeyEnv = "SITE_SESSION_KEY"

type ssmParameterGetter interface {
	GetParameters(ctx context.Context, params *ssm.GetParametersInput, optFns ...func(*ssm.Options)) (*ssm.GetParametersOutput, error)
}

func collectSSMPathEnvVars() (map[string]string, []string) {
	byEnv := make(map[string]string, len(ssmSecretEnvVars))
	paths := make([]string, 0, len(ssmSecretEnvVars))
	for _, name := range ssmSecretEnvVars {
		if val := os.Getenv(name); strings.HasPrefix(val, "/") {
			byEnv[name] = val
			paths = append(paths, val)
		}
	}
	return byEnv, paths
}

func buildPathIndex(out *ssm.GetParametersOutput) map[string]string {
	byPath := make(map[string]string, len(out.Parameters))
	for _, p := range out.Parameters {
		if p.Name != nil && p.Value != nil {
			byPath[*p.Name] = *p.Value
		}
	}
	return byPath
}

func validateSSMSecrets(pathsByEnv, valuesByPath map[string]string) error {
	for _, name := range ssmSecretEnvVars {
		path, ok := pathsByEnv[name]
		if !ok {
			continue
		}
		val, ok := valuesByPath[path]
		if !ok {
			return fmt.Errorf("SSM parameter %q (env %s) not found or inaccessible", path, name)
		}
		if strings.IndexByte(val, 0) >= 0 {
			return fmt.Errorf("SSM parameter %q (env %s) contains an invalid environment value", path, name)
		}
	}
	return nil
}

func applySSMSecrets(pathsByEnv, valuesByPath map[string]string) error {
	for _, name := range ssmSecretEnvVars {
		path, ok := pathsByEnv[name]
		if !ok {
			continue
		}
		val, ok := valuesByPath[path]
		if !ok {
			return fmt.Errorf("SSM parameter %q (env %s) not found or inaccessible", path, name)
		}
		if err := os.Setenv(name, val); err != nil {
			return fmt.Errorf("setenv %s: %w", name, err)
		}
	}
	return nil
}

func resolveRequiredSSMSecretsWithClient(ctx context.Context, client ssmParameterGetter) error {
	pathsByEnv, paths := collectSSMPathEnvVars()
	if len(paths) == 0 {
		return nil
	}
	out, err := client.GetParameters(ctx, &ssm.GetParametersInput{
		Names:          paths,
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("GetParameters: %w", err)
	}
	if out == nil {
		return fmt.Errorf("GetParameters: empty response")
	}
	if len(out.InvalidParameters) > 0 {
		return fmt.Errorf("invalid SSM parameters: %s", strings.Join(out.InvalidParameters, ", "))
	}
	valuesByPath := buildPathIndex(out)
	if err := validateSSMSecrets(pathsByEnv, valuesByPath); err != nil {
		return err
	}
	return applySSMSecrets(pathsByEnv, valuesByPath)
}

func resolveOptionalSessionKeyWithClient(ctx context.Context, client ssmParameterGetter, name, feature string) {
	path := os.Getenv(name)
	if !strings.HasPrefix(path, "/") {
		return
	}
	// Clear the path before fetching so a failure disables only this feature.
	_ = os.Unsetenv(name)
	out, err := client.GetParameters(ctx, &ssm.GetParametersInput{
		Names:          []string{path},
		WithDecryption: aws.Bool(true),
	})
	if err == nil && out != nil && len(out.InvalidParameters) == 0 {
		value, ok := buildPathIndex(out)[path]
		if ok && strings.IndexByte(value, 0) < 0 && os.Setenv(name, value) == nil {
			return
		}
	}
	slog.Warn(feature + " disabled; session key could not be resolved")
}

func resolveSSMSecretsWithClient(ctx context.Context, client ssmParameterGetter) error {
	if err := resolveRequiredSSMSecretsWithClient(ctx, client); err != nil {
		return err
	}
	resolveOptionalSessionKeyWithClient(ctx, client, siteSessionKeyEnv, "site sign-in")
	return nil
}

func clearOptionalSessionPathsOnConfigFailure() {
	if strings.HasPrefix(os.Getenv(siteSessionKeyEnv), "/") {
		_ = os.Unsetenv(siteSessionKeyEnv)
		slog.Warn("site sign-in disabled; session key could not be resolved")
	}
}

// resolveSSMSecrets replaces each env var in ssmSecretEnvVars whose current
// value begins with "/" with the decrypted value fetched from AWS SSM Parameter
// Store. This keeps plaintext secrets out of Terraform state while still making
// them available to the application via the standard os.Getenv API. The optional
// site session key is resolved separately so its failure cannot stop the public
// application from starting.
func resolveSSMSecrets(ctx context.Context) error {
	_, paths := collectSSMPathEnvVars()
	sitePath := strings.HasPrefix(os.Getenv(siteSessionKeyEnv), "/")
	if len(paths) == 0 && !sitePath {
		return nil
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		if len(paths) > 0 {
			return fmt.Errorf("load AWS config: %w", err)
		}
		clearOptionalSessionPathsOnConfigFailure()
		return nil
	}
	return resolveSSMSecretsWithClient(ctx, ssm.NewFromConfig(cfg))
}
