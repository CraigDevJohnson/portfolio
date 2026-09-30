# This root is retired. Its management-account resources are destroyed from the
# management-final tag (refactor Phase 14), which does not have this guard.
# Here it points at the workloads backend, where a plan would recreate the
# legacy ECR repository, tables and policies, so every plan fails instead.
# validate still passes. Phase 14 deletes this root.
locals {
  # A precondition must reference something, so the guard reads this constant.
  root_retired = true
}

resource "terraform_data" "retired" {
  lifecycle {
    precondition {
      condition     = !local.root_retired
      error_message = "The legacy infra root is retired. Never plan or apply it; see DEPLOY-INSTRUCTIONS.md."
    }
  }
}
