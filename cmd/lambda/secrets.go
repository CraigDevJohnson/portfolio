package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// ssmSecretEnvVars lists the environment variable names whose values are SSM
// parameter paths that must be resolved to their plaintext secrets before the
// application configuration is loaded.
var ssmSecretEnvVars = []string{"CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "LPS_SESSION_KEY"}

func collectSSMPaths() []string {
	paths := make([]string, 0, len(ssmSecretEnvVars))
	for _, name := range ssmSecretEnvVars {
		if val := os.Getenv(name); strings.HasPrefix(val, "/") {
			paths = append(paths, val)
		}
	}
	return paths
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

func applySSMSecrets(byPath map[string]string) error {
	for _, name := range ssmSecretEnvVars {
		path := os.Getenv(name)
		if !strings.HasPrefix(path, "/") {
			continue
		}
		val, ok := byPath[path]
		if !ok {
			return fmt.Errorf("SSM parameter %q (env %s) not found or inaccessible", path, name)
		}
		if err := os.Setenv(name, val); err != nil {
			return fmt.Errorf("setenv %s: %w", name, err)
		}
	}
	return nil
}

// resolveSSMSecrets replaces each env var in ssmSecretEnvVars whose current
// value begins with "/" with the decrypted value fetched from AWS SSM Parameter
// Store. This keeps plaintext secrets out of Terraform state while still making
// them available to the application via the standard os.Getenv API.
func resolveSSMSecrets(ctx context.Context) error {
	paths := collectSSMPaths()
	if len(paths) == 0 {
		return nil
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}
	client := ssm.NewFromConfig(cfg)
	out, err := client.GetParameters(ctx, &ssm.GetParametersInput{
		Names:          paths,
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("GetParameters: %w", err)
	}
	byPath := buildPathIndex(out)
	return applySSMSecrets(byPath)
}
