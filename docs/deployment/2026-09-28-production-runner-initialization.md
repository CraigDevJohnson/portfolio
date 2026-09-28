# Production apply runner initialization

Release 36379213073 failed before infrastructure application because the fresh
production-apply runner had no cached AWS provider when the validator rendered
the saved binary plan. Protected approval and identity checks had succeeded.
Deployment 6702658441 correctly records failure; the public site retained the
previous bootstrap source. Its evidence is retained, not relabeled as success.

Initialize the exact, checksum-bound production backend with the checked-in
provider lock file after approval, current-main, artifact and override checks.
Require the default workspace before rendering and validating the saved plan.
Keep the same verified empty CLI configuration on the final apply invocation;
a child shell export alone does not propagate that setting to its parent.
The existing final approval, alias/image, current-main and alarm-route checks
still run before apply. No permissions, resources, runtime image or session key
change is introduced by this correction.

Regression fixtures require initialization before plan rendering and reject
initialization failure and a non-default workspace. The next attempt requires a
fresh manifest-only promotion, plan and protected approval; do not rerun the old
release attempt or reuse its approval.
