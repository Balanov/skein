# Security

## Guarantee boundary

Skein provides fail-closed scheduling for its own local runs: a persistent stop latch, epoch-fenced queued work, rejection of late completions, and termination of tracked local process trees. It does not make an agent truthful, a tool call semantically correct, or an arbitrary command safe.

A fencing lease is not a signed identity token. Do not expose it as an authentication mechanism. Do not assume an external provider honors it unless a separate enforcing gateway checks it.

Skein is not a sandbox. Manifest commands execute with the user's permissions and may access anything that user can access. Review manifests, use argv rather than shell strings, constrain workers and passes, and run untrusted workloads in an isolated OS environment.

Control directories may contain prompts, outputs, errors, and other sensitive data. They are created with owner-only permissions where the platform supports them; protect and delete them according to your data policy. Event logs intentionally exclude payload contents but may contain task IDs and error strings.

## Reporting a vulnerability

Open a private GitHub security advisory for `Balanov/skein` rather than publishing exploitable details in an issue. Include the affected version/commit, operating system, reproduction steps, and the guaranteed boundary you believe is crossed.
