# Saddlebag Architecture Overview

> **Saddlebag** is the developer environment control plane and policy spine of the Packmule platform.
> It is a macOS menu bar app (`Saddlebag.app`) paired with a Go CLI companion (`sb`).
>
> **Two roles:**
> 1. **Operations hub** — `sb` installs, configures, and manages the lifecycle of every Packmule component (packmule, byobrain, campfire). It is the portable developer toolbelt.
> 2. **Policy proxy** — at runtime, all LLM requests from BYOBrain flow through saddlebag's gateway (:7474), which enforces identity, budget, and Cedar policies before forwarding to packmule (:7475).

---

## Table of Contents

1. [Current State: What Exists Today](#1-current-state-what-exists-today)
2. [Component Map](#2-component-map)
3. [Process & Communication Model](#3-process--communication-model)
4. [Key Subsystems](#4-key-subsystems)
   - [Desk Resolution](#41-desk-resolution)
   - [Policy Proxy (Gateway)](#42-policy-proxy-gateway)
   - [Component Lifecycle Management](#43-component-lifecycle-management)
   - [IPC Channel](#44-ipc-channel)
   - [Secrets Manager Integration](#45-secrets-manager-integration)
   - [Credential Health](#46-credential-health)
5. [File & Config Layout](#5-file--config-layout)
6. [Runtime Architecture](#6-runtime-architecture)
7. [Conventions & Design Decisions](#7-conventions--design-decisions)

---

## 1. Current State: What Exists Today

Saddlebag is split into three distinct targets that live in this repo:

| Target | Language | Role |
|---|---|---|
| `Saddlebag.app` | Swift 6 / SwiftUI | macOS menu bar app; credential UI, IPC server |
| `SaddlebagShared` | Swift (local package) | Models and services shared between app targets |
| `sb` | Go | CLI companion; shell integration, desk switching, secrets, policy proxy, lifecycle management |

The Swift app and `sb` communicate over a **Unix domain socket** at `/tmp/saddlebag.sock`.

---

## 2. Component Map

```
┌──────────────────────────────────────────────────────────────────┐
│  User Surfaces                                                   │
│  macOS Menu Bar (Saddlebag.app)  ·  Terminal (sb CLI)            │
├──────────────────────────────────────────────────────────────────┤
│  SaddlebagShared (Swift local package)                           │
│  Models: AWSProfile, GCPConfiguration, Desk, Secret, …          │
│  Services: AWSConfigService, GCPConfigService,                   │
│            DeskService, SecretManagerService,                    │
│            IPCServer, CredentialHealthMonitor, Obfuscator        │
├────────────────────────┬─────────────────────────────────────────┤
│  Saddlebag.app         │  sb (Go CLI)                            │
│  ViewModels/           │  cmd/: desk, env, secrets, gateway,     │
│    AccountsViewModel   │        pm, brain, campfire, health,     │
│    SecretsViewModel    │        ledger, policy, switch           │
│  Views/                │  internal/:                             │
│    MenuBarView         │    config/ · desk/ · health/            │
│    MainAppView         │    ipc/ · secrets/ · state/             │
│    SecretsView         │    gateway/ · policy/ · ledger/         │
│    SettingsView        │    process/ (lifecycle manager)         │
├────────────────────────┴─────────────────────────────────────────┤
│  Runtime Services (sb manages these)                             │
│  sb gateway :7474  — policy proxy (budget, Cedar, identity)      │
│  pm serve :7475    — LLM gateway (packmule, managed by sb)       │
│  byobrain stdio    — MCP server + :7476 sidecar                  │
│  campfire :3000    — Nuxt 4 UI                                   │
└──────────────────────────────────────────────────────────────────┘
```

---

## 3. Process & Communication Model

```
sb CLI  ──[JSON-RPC over Unix socket]──▶  Saddlebag.app (IPCServer)
                                                 │
                                          DeskService
                                          AWSConfigService
                                          GCPConfigService
                                          CredentialHealthMonitor

BYOBrain  ──[HTTP :7474]──▶  sb gateway (policy proxy)
                                    │ budget check
                                    │ Cedar policy check
                                    │ routing resolution
                                    ▼
                             pm serve :7475 (LLM gateway)
                                    │
                                    ▼
                             LLM providers (Anthropic, OpenAI, …)
```

---

## 4. Key Subsystems

### 4.1 Desk Resolution

A **desk** is a named environment bundle: AWS profile + GCP config + git identity + shell variables + LLM routing config. The CLI resolves the active desk via a strict four-tier priority chain:

```
$SADDLEBAG_DESK (env var)          ← Tier 1: shell override
  └── .desk file (walk up CWD)     ← Tier 2: local pin
        └── workdir match          ← Tier 3: desk's configured working dir
              └── state.json       ← Tier 4: global default
```

Relevant code:
- [`sb/cmd/desk.go`](../sb/cmd/desk.go) — `resolveCurrentDesk()` implements the chain
- [`sb/internal/desk/resolve.go`](../sb/internal/desk/resolve.go) — `FindDeskFile`, `FindDeskByWorkingDir`
- [`sb/internal/desk/desk.go`](../sb/internal/desk/desk.go) — `LoadAll`, `WriteDeskFile`

Desk definitions live at `~/.saddlebag/desks/*.toml`.

### 4.2 Policy Proxy (Gateway)

`sb gateway start` runs an HTTP server on `:7474` that acts as a **policy proxy** in front of packmule.

**Every LLM request from BYOBrain flows through here.** The proxy:

1. Parses the OpenAI-compatible request
2. Checks **budget** — reads today's ledger spend; returns 429 if over limit
3. Checks **Cedar policy** — evaluates SDLC constraints (e.g., `enter_execution` requires approved plan)
4. Resolves **routing** from desk config (`X-Task-Type → provider/model`)
5. Adds headers: `X-Provider`, `X-Model`, `X-Budget-Remaining`, `traceparent`
6. **Reverse-proxies** to `pm serve :7475`
7. On response: appends token usage to the **ledger**

Saddlebag never touches provider wire format. It is a thin, fast policy checkpoint.

**Additional endpoints:**
```
POST /v1/policy/check    # Cedar policy check (called by BYOB before state transitions)
GET  /v1/ledger          # today's ledger summary (called by BYOB's get_budget_status tool)
GET  /health
```

Relevant code:
- [`sb/internal/gateway/server.go`](../sb/internal/gateway/server.go) — policy proxy server
- [`sb/internal/gateway/proxy.go`](../sb/internal/gateway/proxy.go) — reverse proxy to packmule
- [`sb/internal/gateway/budget.go`](../sb/internal/gateway/budget.go) — budget enforcement
- [`sb/internal/policy/cedar.go`](../sb/internal/policy/cedar.go) — Cedar stub (Phase 1a: file-based; Phase 1b: full Cedar)
- [`sb/internal/ledger/`](../sb/internal/ledger/) — immutable JSONL token ledger

### 4.3 Component Lifecycle Management

`sb` is the operations hub for the entire Packmule stack:

```bash
# Packmule LLM gateway
sb pm install          # go build → ~/.saddlebag/bin/pm
sb pm start            # pm serve :7475 (background daemon with PID file)
sb pm stop
sb pm status

# BYOBrain MCP server
sb brain install       # npm install byobrain-mcp
                       # write MCP config to:
                       #   ~/Library/Application Support/Claude/claude_desktop_config.json
                       #   ~/.cursor/mcp.json
                       #   ~/.gemini/config.json
sb brain status

# Campfire UI
sb campfire serve      # nuxt start (background)
sb campfire open       # open browser to campfire URL

# Saddlebag policy proxy
sb gateway start       # sb policy proxy :7474 (background daemon)
sb gateway stop
sb gateway status
```

Background daemons use macOS launchd (`~/Library/LaunchAgents/com.packmule.{component}.plist`) generated by `sb`. Each component can also be run in the foreground for development.

### 4.4 IPC Channel

`Saddlebag.app` runs `IPCServer` as a background actor listening on the Unix socket. When `sb` calls `ipc.Send`, the app receives the `Command`, dispatches to `DeskService` (or other services), and returns a `Response`.

Relevant code:
- [`sb/internal/ipc/client.go`](../sb/internal/ipc/client.go) — Go client
- [`SaddlebagShared/Sources/Services/IPCServer.swift`](../SaddlebagShared/Sources/Services/IPCServer.swift) — Swift server

### 4.5 Secrets Manager Integration

Saddlebag reads secrets from **GCP Secret Manager** using a label convention. Labels (`org`, `service`, `stage`) drive `.env` file generation.

At runtime, the policy proxy fetches API keys from the secret store and passes them to packmule per-request via request headers — API keys are never stored in packmule.

Relevant code:
- [`sb/cmd/secrets.go`](../sb/cmd/secrets.go) — CLI commands
- [`SaddlebagShared/Sources/Services/SecretManagerService.swift`](../SaddlebagShared/Sources/Services/SecretManagerService.swift) — app-side GCP calls

### 4.6 Credential Health

`CredentialHealthMonitor` (Swift) watches AWS SSO token expiry and GCP credential state, emitting macOS notifications when credentials transition between states (🟢 active → 🟡 expiring → 🔴 expired).

Relevant code:
- [`SaddlebagShared/Sources/Services/CredentialHealthMonitor.swift`](../SaddlebagShared/Sources/Services/CredentialHealthMonitor.swift)
- [`sb/cmd/health.go`](../sb/cmd/health.go), [`sb/internal/health/`](../sb/internal/health/)

---

## 5. File & Config Layout

```
~/.saddlebag/
├── config.json          # user preferences
├── state.json           # live state: active desk, AWS profile, GCP config, git email
├── bin/
│   └── pm               # packmule binary (installed by sb pm install)
├── desks/
│   └── <name>.toml      # one per desk context
└── ledger/
    └── YYYY-MM-DD.jsonl # daily token ledger (immutable append)

~/Library/LaunchAgents/
├── com.packmule.gateway.plist    # sb gateway start daemon
└── com.packmule.pm.plist         # sb pm start daemon

/tmp/saddlebag.sock      # Unix socket (app IPC server)
~/.aws/config            # AWS profiles (read-only by Saddlebag)
~/.config/gcloud/        # gcloud configs (read via `gcloud` CLI invocations)
```

Desk TOML schema (example):
```toml
[aws]
profile = "courseclear-prod"

[gcp]
config = "courseclear"

[git]
email = "marcus@courseclear.com"

[llm]
default_provider = "anthropic"
default_model    = "claude-sonnet-4-20250514"

[llm.routing]
plan      = "claude-opus-4-20250514"
implement = "claude-sonnet-4-20250514"
classify  = "ollama/gemma3:2b"

[llm.budget]
daily_limit_usd = 50.00
warn_at_usd     = 40.00

[workdir]
path = "/Users/marcus/dev/courseclear"
```

---

## 6. Runtime Architecture

```
call flow: BYOBrain → saddlebag :7474 → packmule :7475 → LLM APIs

saddlebag's role at :7474:
  ┌─────────────────────────────────────────────────────┐
  │  1. Parse request                                   │
  │  2. Budget check → 429 if exceeded                  │
  │  3. Cedar policy check → 403 if denied              │
  │  4. Resolve routing (desk config → X-Provider/Model)│
  │  5. Inject API key header (from secret store)       │
  │  6. Add traceparent, budget headers                 │
  │  7. Reverse proxy → packmule :7475                  │
  │  8. Record response to ledger                       │
  └─────────────────────────────────────────────────────┘
```

Saddlebag does **not** implement provider adapters, streaming SSE bodies, or LLM wire formats.
That is packmule's job.

---

## 7. Conventions & Design Decisions

| Decision | Rationale |
|---|---|
| **`~/.saddlebag/` for all user data** | Explicit, portable, not buried in `~/Library/Application Support/` |
| **`sb` as master lifecycle manager** | One tool that installs, configures, and runs the entire stack. Portable developer toolbelt. |
| **Policy proxy at :7474, LLM gateway at :7475** | Clean separation: BYOB always calls :7474 (unchanged if packmule moves). Saddlebag owns the policy checkpoint. |
| **API keys injected per-request, never stored in packmule** | Keys are saddlebag's responsibility. Packmule is ephemeral — it can be restarted without secret re-configuration. |
| **Cedar stub in Phase 1a (file-based)** | Validates the workflow before adding full Cedar evaluation overhead. Phase 1b replaces with cedar-go. |
| **launchd for background daemons** | macOS-native; survives reboots; no separate daemon manager. |
| **Go CLI (`sb`)** | Cross-compilation, fast startup, no runtime dependency — ideal for shell integration (`eval "$(sb init zsh)"`). |
| **Unix socket IPC (not REST/gRPC)** | Localhost-only, no port management, peer credential verification via `LOCAL_PEERCRED`, zero network attack surface. |
| **Go module path `github.com/marcusguttenplan/sb`** | Matches the open-source repo; ready for extraction if CLI is split. |

---

*Last updated: June 2026. Reflects the agreed architecture: packmule owns the LLM gateway, saddlebag owns policy and developer operations.*
