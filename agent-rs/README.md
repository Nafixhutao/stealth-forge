# Stealth Agent (Rust)

The trusted runtime behind Stealth's Agent runs. The Go worker owns the durable
queue, leases, logs, cancellation, and result persistence. This binary receives
one bounded job on stdin, runs a tool-calling loop against an OpenAI-compatible
model, executes tools inside a hardened sandbox, and writes one bounded result
to stdout.

## Process contract

**stdin** — one JSON object:

```json
{ "run": { "id": "...", "prompt": "..." }, "agent": { "id": "...", "instructions": null } }
```

**stdout** — one JSON object:

```json
{
  "output_text": "…",
  "error_message": null,
  "steps": [{ "id": "call_1", "type": "command", "label": "shell", "target": "…", "status": "done" }],
  "changes": [{ "path": "letter.docx", "additions": 0, "deletions": 0, "status": "added" }],
  "files": [{ "path": "letter.docx", "mime_type": "…", "size_bytes": 1234 }]
}
```

Logs go to stderr and are forwarded to `agent_run_logs` by the Go adapter. A
structured failure is returned on stdout with `error_message` set, so the run
persists its own message instead of a generic worker error.

## Tools

| Tool | Purpose |
| --- | --- |
| `shell` | Run `sh -lc` inside the sandbox workdir. |
| `write_file` | Write a UTF-8 file inside the workdir. |
| `read_file` | Read a UTF-8 file inside the workdir. |

Paths are relative to the workdir; absolute paths and `..` are rejected.

## Sandbox

One Docker container per run:

- `--network=none`, read-only root filesystem, one writable volume.
- `--cap-drop ALL`, `no-new-privileges`, non-root (`65532:65532`).
- Memory, CPU, PID, and command-time limits.
- The image (`sandbox/Dockerfile`) carries Python, `python-docx`, LibreOffice,
  and fonts.

The run directory lives under `AGENT_WORK_ROOT` in the worker's
`stealth_function_runner_staging` volume. The container mounts the same volume,
so it sees the files the worker writes. The directory is kept after the run so
the artifact pipeline can collect the documents.

## Configuration

| Variable | Default | Notes |
| --- | --- | --- |
| `AGENT_LLM_BASE_URL` | `https://api.rinsdev.cloud/v1` | OpenAI-compatible endpoint. |
| `AGENT_LLM_MODEL` | `deepseek-v4.1-flash-1017` | Model used by the agent. |
| `AGENT_LLM_API_KEY` | — | Required. Never logged. |
| `AGENT_MAX_STEPS` | `12` | Loop bound. |
| `AGENT_MAX_TOKENS` | `4096` | Per completion. |
| `AGENT_REQUEST_TIMEOUT_SECS` | `120` | Per LLM request. |
| `AGENT_SANDBOX_ENABLED` | `true` | Local fallback when `false`. |
| `AGENT_SANDBOX_IMAGE` | `stealth-agent-sandbox:latest` | Sandbox image. |
| `AGENT_SANDBOX_MEMORY` / `CPUS` / `PIDS` | `512m` / `1` / `128` | Container limits. |
| `AGENT_SANDBOX_TIMEOUT_SECS` | `120` | Per command. |
| `AGENT_WORK_ROOT` | `/var/lib/stealth/runner-staging` | Shared volume mount. |
| `AGENT_SANDBOX_VOLUME` | `stealth_function_runner_staging` | Named volume. |
| `AGENT_SANDBOX_VOLUME_WORKDIR` | `/workspace` | Volume mount inside the sandbox. |

## Build

```bash
cargo test
cargo build --release
docker build -f sandbox/Dockerfile -t stealth-agent-sandbox:latest .
```

The repository `Dockerfile` builds this crate in a `rust-build` stage and copies
`stealth-agent` into the worker image.
