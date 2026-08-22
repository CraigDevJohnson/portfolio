package lambda

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestLambdaInfrastructureLayout(t *testing.T) {
	required := []string{
		"artifacts/backend.hcl",
		"artifacts/main.tf",
	}

	for _, path := range required {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("required Lambda infrastructure path %q: %v", path, err)
		}
	}

	for _, path := range []string{
		"infra/lambda/artifacts/.terraform/providers/example",
		"infra/lambda/artifacts/.tofu/providers/example",
		"infra/lambda/artifacts/terraform.tfstate",
		"infra/lambda/artifacts/terraform.tfstate.backup",
		"infra/lambda/artifacts/saved.tfplan",
	} {
		if !gitPathIsIgnored(t, path) {
			t.Errorf("generated OpenTofu path %q must be ignored", path)
		}
	}

	if gitPathIsIgnored(t, "infra/lambda/artifacts/.terraform.lock.hcl") {
		t.Error("the artifact root provider lock file must not be ignored")
	}
}

func gitPathIsIgnored(t *testing.T, path string) bool {
	t.Helper()

	command := exec.Command("git", "check-ignore", "--no-index", "--quiet", path)
	command.Dir = "../.."
	err := command.Run()
	if err == nil {
		return true
	}

	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false
	}

	t.Fatalf("check whether %q is ignored: %v", path, err)
	return false
}
