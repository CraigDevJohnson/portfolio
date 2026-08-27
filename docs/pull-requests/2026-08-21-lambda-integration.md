## Summary

- consolidate PRs #39 and #42 plus audited cleanup `c5419d40` on current `main`
- preserve current-main reminder behavior and the useful time-relative fixtures from #43
- make API Gateway HTTPS, cookies, Lambda initialization, long Google work, and image builds release-safe
- add a real repository CI gate

## Source audit

- base: `24ab9ace530e6dd8a1736f34e4f078afc63e480b`
- PR #39 head: `005cc87da0b0b1a2c536a9b0ed53c3d936a9bc38`
- PR #42 head: `3f17a0c1a9f1915cf2fe6837676940e40fdec77c`
- cleanup candidate: `c5419d40716796531ad5da0dd836bb05e0adb010`
- expected modify/delete conflicts resolved by retaining the candidate's modular deletion of root `main.go` and `main_test.go`

## Verification

- `task ci`
- targeted race tests for Lambda, HTTP origin, Google, Soccer, and application packages
- repeated Google sync and schedule tests
- `go mod tidy -diff` and `go mod verify`
- Linux amd64 Lambda bootstrap inspection
- Linux amd64 regular-server inspection and runtime CA contract

## Deployment

Opening or merging this PR does not deploy AWS infrastructure or change DNS.
