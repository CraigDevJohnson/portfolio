# Codex Cloud Environment Research

**Scope:** current, documented Codex cloud-environment configuration behavior. This is a reference note, not a repository setup script. Verified 2026-08-31 against [Cloud environments](https://learn.chatgpt.com/docs/environments/cloud-environment) and [Agent internet access](https://learn.chatgpt.com/docs/cloud/internet-access).

## Documented settings and actions

| Setting or action | Semantics and constraints |
| --- | --- |
| **Set package versions** | Pins Python, Node.js, and other runtime versions in the default `universal` image. Use it when the project needs a non-default runtime; install anything else with setup. |
| **Automatic setup** | Codex can install dependencies/tools for projects using `npm`, `yarn`, `pnpm`, `pip`, `pipenv`, or `poetry`. |
| **Setup script** | Runs for a newly prepared cached container, after the repository is cloned at its default branch. It is the place for complex dependency/tool installation and has internet access. It runs in a separate Bash session, so `export` values do not carry into the agent phase. Persist a non-secret value through environment settings or `~/.bashrc`. |
| **Maintenance script** (optional) | Runs only when Codex resumes a cached container, after it checks out the branch selected for that chat. Use it to refresh state that can become stale between the default-branch setup commit and the chat branch. |
| **Environment variables** | Available for the whole chat, including setup and the agent phase. They are appropriate for non-sensitive configuration. |
| **Secrets** | Stored with additional encryption and decrypted only for task execution, but exposed only to setup scripts and removed before the agent phase. Do not use a secret for an agent-time credential; do not write secret values into setup output or files that survive into the agent phase. |
| **Agent internet access** | Off by default. When enabled per environment, select an allowlist preset: **None**, **Common dependencies**, or **All (unrestricted)**. With None or Common dependencies, add only required domains. Limit allowed methods to `GET`, `HEAD`, and `OPTIONS` where possible; non-read methods otherwise expand the ability to transmit data. |
| **Reset cache** | Use when repository changes make cached state incompatible. Changing setup, maintenance, variables, or secrets invalidates the cache automatically. Business and Enterprise environment caches are shared with everyone who can access that environment, so invalidation affects them too. |

The documentation does not describe other UI fields (for example, a display name or description) or field-level input validation. Treat the table as the complete set of documented configuration controls, not a stable inventory of every control currently rendered by the authenticated settings UI.

## Execution timeline and runtime assumptions

1. Submitting a cloud-chat prompt creates a container and checks out the repository at the selected branch or commit SHA.
2. Codex runs setup; when a cached container is resumed, it additionally runs the optional maintenance script.
3. Setup always has internet access. The agent phase then uses the environment's internet setting.
4. The agent edits and validates in a terminal loop. It consults repository `AGENTS.md` guidance when present.
5. Codex returns a response and diff; the user can open a PR or continue the chat.

The base is Codex's `universal` container, preloaded with common languages, packages, and tools. It is a convenience baseline rather than a substitute for pinning the runtime and tools required by this repository. Cached container state lasts up to 12 hours: initial setup uses the default branch, while resumed chats check out the requested branch before maintenance.

## Security and network posture

Keep agent internet access disabled unless the task requires it. If it does, prefer a minimal domain allowlist and read-only methods. OpenAI identifies prompt injection, code/secret exfiltration, malicious or vulnerable downloads, and licensing-restricted material as concrete risks. Review the work log, diff, and agent output before accepting changes.

All outbound traffic passes through an HTTP/HTTPS proxy. Setup's network access is deliberately broader in timing—it is available so dependencies can be installed—so setup scripts should install only pinned or otherwise trusted dependencies and must not print credentials.

## Configuration recommendation for this repository

The saved environment uses the `universal` image, a manual setup script with pinned and checksum-verified Go/Task/Templ/golangci-lint/OpenTofu tooling, and post-setup caching. Its maintenance script refreshes Go modules and the pinned Tailwind binary after the task branch is checked out, then revalidates tool versions. It defines no environment variables or secrets.

Agent internet is **On** with the **Common dependencies** preset and only `GET`, `HEAD`, and `OPTIONS`. That permits read-only dependency and public-source lookups needed by this repository's evidence-led workflow while excluding write methods and unrestricted domains. Tighten it to **Off** for tasks that need no agent-time network access.

## Source limitations and UI-drift note

The official docs describe behavior and named controls, but do not publish a complete authenticated-settings form schema, field validation rules, or an availability matrix by plan. The unauthenticated Codex settings link redirects to the ChatGPT landing page, so field labels and auxiliary UI controls should be rechecked in the signed-in product before rollout. Do not infer undocumented size limits, script timeouts, package-version options, or secret-name rules from this note.

## Official sources

- [Cloud environments](https://learn.chatgpt.com/docs/environments/cloud-environment)
- [Agent internet access](https://learn.chatgpt.com/docs/cloud/internet-access)
