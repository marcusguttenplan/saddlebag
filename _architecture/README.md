# Saddlebag Architecture Overview

> **Saddlebag** is the "spine" of the Packmule platform — a credential and developer-context control plane that lives as a macOS menu bar app (`Saddlebag.app`) paired with a Go CLI companion (`sb`). It manages multi-cloud credential profiles, desk context switching, and secrets, and will evolve into the gateway, IAM, audit, and sandboxing layer for the full Packmule harness.

---

## Table of Contents

1. [Current State: What Exists Today](#1-current-state-what-exists-today)
2. [Component Map](#2-component-map)
3. [Process & Communication Model](#3-process--communication-model)
4. [Key Subsystems](#4-key-subsystems)
   - [Desk Resolution](#41-desk-resolution)
   - [IPC Channel](#42-ipc-channel)
   - [Secrets Manager Integration](#43-secrets-manager-integration)
   - [Credential Health](#44-credential-health)
5. [File & Config Layout](#5-file--config-layout)
6. [Future Direction: Packmule Platform](#6-future-direction-packmule-platform)
7. [Conventions & Design Decisions](#7-conventions--design-decisions)

---

## 1. Current State: What Exists Today

Saddlebag is split into three distinct targets that live in this repo:

| Target | Language | Role |
|---|---|---|
| `Saddlebag.app` | Swift 6 / SwiftUI | macOS menu bar app; credential UI, IPC server |
| `SaddlebagShared` | Swift (local package) | Models and services shared between app targets |
| `sb` | Go | CLI companion; shell integration, desk switching, secrets |

The two sides communicate over a **Unix domain socket** at `/tmp/saddlebag.sock`. The Swift app acts as the server; `sb` is the client.

---

## 2. Component Map

```
┌──────────────────────────────────────────────────────────────┐
│  User Surfaces                                               │
│  macOS Menu Bar (Saddlebag.app)  ·  Terminal (sb CLI)        │
├──────────────────────────────────────────────────────────────┤
│  SaddlebagShared (Swift local package)                       │
│  Models: AWSProfile, GCPConfiguration, Desk, Secret, …      │
│  Services: AWSConfigService, GCPConfigService,               │
│            DeskService, SecretManagerService,                │
│            IPCServer, CredentialHealthMonitor, Obfuscator    │
├────────────────────────┬─────────────────────────────────────┤
│  Saddlebag.app         │  sb (Go CLI)                        │
│  ViewModels/           │  cmd/: desk, env, secrets,          │
│    AccountsViewModel   │        switch, health, doctor,      │
│    SecretsViewModel    │        gitcheck, setup, use         │
│  Views/                │  internal/:                         │
│    MenuBarView         │    config/ · desk/ · health/        │
│    MainAppView         │    ipc/ · secrets/ · state/         │
│    SecretsView         │                                     │
│    SettingsView        │                                     │
├────────────────────────┴─────────────────────────────────────┤
│  IPC Layer                                                   │
│  Unix socket: /tmp/saddlebag.sock                            │
│  Protocol: newline-delimited JSON (Command → Response)       │
├──────────────────────────────────────────────────────────────┤
│  External State / Cloud                                      │
│  ~/.saddlebag/     ~/.aws/config     gcloud configurations   │
│  GCP Secret Manager                                          │
└──────────────────────────────────────────────────────────────┘
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
```

**Key protocol details:**

- Socket path: `/tmp/saddlebag.sock` (derived from `config.SocketPath()`)
- Message format: newline-delimited JSON; one `Command` per connection, one `Response` back
- Timeout: 5 seconds (enforced by `ipc.Send` via `context.WithTimeout`)
- Fallback: if the app is not running, `sb` writes state directly to `~/.saddlebag/state.json`

```go
// ipc/client.go: the entire wire protocol in ~30 lines
type Command  struct { Action string; Desk string }
type Response struct { Status, Message, Error string }
```

---

## 4. Key Subsystems

### 4.1 Desk Resolution

A **desk** is a named environment bundle: AWS profile + GCP config + git identity + shell variables. The CLI resolves the active desk via a strict four-tier priority chain:

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
- [`SaddlebagShared/Sources/Services/DeskService.swift`](../SaddlebagShared/Sources/Services/DeskService.swift) — app-side switching (applies AWS/GCP/SSH, writes state)

Desk definitions live at `~/.saddlebag/desks/*.toml`.

### 4.2 IPC Channel

`Saddlebag.app` runs `IPCServer` as a background actor listening on the Unix socket. When `sb` calls `ipc.Send`, the app receives the `Command`, dispatches to `DeskService` (or other services), and returns a `Response`.

The socket is the **only** integration point between the Go CLI and the Swift app. There is no shared memory, no file locking, and no polling — the CLI writes state directly only when the app is confirmed not running.

Relevant code:
- [`sb/internal/ipc/client.go`](../sb/internal/ipc/client.go) — Go client
- [`SaddlebagShared/Sources/Services/IPCServer.swift`](../SaddlebagShared/Sources/Services/IPCServer.swift) — Swift server

### 4.3 Secrets Manager Integration

Saddlebag reads secrets from **GCP Secret Manager** using a label convention. Labels (`org`, `service`, `stage`) drive `.env` file generation — the secret name becomes the env var name.

| Label | Role |
|---|---|
| `org` | Grouping / client |
| `service` | Maps to a subdirectory in generated output |
| `stage` | Maps to the file suffix (`.env.prod`, `.env.dev`) |

The CLI surface (`sb secrets list/get/env/create/tag/copy`) mirrors the UI's Secrets tab in the main app window.

Relevant code:
- [`sb/cmd/secrets.go`](../sb/cmd/secrets.go) — CLI commands
- [`SaddlebagShared/Sources/Services/SecretManagerService.swift`](../SaddlebagShared/Sources/Services/SecretManagerService.swift) — app-side GCP calls

### 4.4 Credential Health

`CredentialHealthMonitor` (Swift) watches AWS SSO token expiry and GCP credential state, emitting macOS notifications when credentials transition between states (🟢 active → 🟡 expiring → 🔴 expired). This is background-monitored; `sb health` provides a CLI snapshot.

Relevant code:
- [`SaddlebagShared/Sources/Services/CredentialHealthMonitor.swift`](../SaddlebagShared/Sources/Services/CredentialHealthMonitor.swift)
- [`sb/cmd/health.go`](../sb/cmd/health.go), [`sb/internal/health/`](../sb/internal/health/)

---

## 5. File & Config Layout

```
~/.saddlebag/
├── config.json          # user preferences (labels, tags, favorites, obfuscation)
├── state.json           # live state: active desk, AWS profile, GCP config, git email
└── desks/
    └── <name>.toml      # one per desk context

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

[env]
SOME_VAR = "value"

[workdir]
path = "/Users/marcus/dev/courseclear"
```

---

## 6. Future Direction: Packmule Platform

Saddlebag is the **spine** of the planned Packmule harness. The roadmap adds the following capabilities to this repo (primarily to `sb`):

### Phase 1 additions (immediate)

| Component | Where | What |
|---|---|---|
| **Model Gateway** (`sb gateway`) | `sb/cmd/gateway.go` + `sb/internal/gateway/` | Localhost OpenAI-compatible proxy with hierarchical routing, token ledger, budget enforcement |
| **Cedar Policy Engine** (`sb policy`) | `sb/internal/policy/` | Deny-by-default capability containment; `sb policy check/explain/test/lint` |
| **Sandbox Orchestrator** | `sb/internal/sandbox/` | Seatbelt profile generation; wraps agent processes |
| **Human Gate** | Menu bar + `sb approve` | macOS notification → approve/deny → Cedar rule generation |
| **Audit Log + Token Ledger** | `sb/internal/audit/` | Append-only JSONL; `X-Trace` correlation IDs |

### Layered architecture target

```
┌─────────────────────────────────────────────────────────────┐
│  Surface Layer                                              │
│  IDE (via MCP) · TUI · Campfire (Nuxt 4 web UI)            │
├─────────────────────────────────────────────────────────────┤
│  byobrain (Node/TS)  — the Brain                            │
│  Agent loop · Spec SDLC · Context · Skills · Sub-agents     │
├─────────────────────────────────────────────────────────────┤
│  saddlebag (Go + Swift)  — the Spine  ◀ this repo          │
│  Model gateway · Cedar IAM · Sandbox · Credentials          │
│  Audit log · Token ledger · Metrics · Monitoring            │
├─────────────────────────────────────────────────────────────┤
│  Packmule Services                                          │
│  Neo4j graph · Brain sync · Event bus · CRDT state          │
│  Presence · Identity (local → OIDC)                         │
└─────────────────────────────────────────────────────────────┘
```

**Design thesis:** byobrain decides *what* to do; saddlebag decides *whether* it's allowed and *with what*; packmule services make it multiplayer, observable, and persistent.

**Three non-negotiables:**
1. **Brain/spine separation is physically enforced** — different runtimes, different processes, socket boundary. Reasoning never holds keys.
2. **Event-sourcing as the unifying model** — if you cannot rebuild it from canonical truth, do not trust it.
3. **Plan approval as a credential precondition** — spec-discipline is enforced by Cedar policy, not prompt engineering.

See [`packmule_plan.md`](../../Downloads/packmule_plan.md) for the full HLD/PRD.

---

## 7. Conventions & Design Decisions

| Decision | Rationale |
|---|---|
| **`~/.saddlebag/` for all user data** | Explicit, portable, not buried in `~/Library/Application Support/` |
| **Xcode project (not SPM workspace root)** | Enables Widget Extension targets, App Groups, and proper code signing |
| **`SaddlebagShared` as a local Swift package** | Clean separation; shared between app and future widget/extension targets without code duplication |
| **Go CLI (`sb`)** | Cross-compilation, fast startup, no runtime dependency — ideal for shell integration (`eval "$(sb init zsh)"`) |
| **Unix socket IPC (not REST/gRPC)** | Localhost-only, no port management, peer credential verification via `LOCAL_PEERCRED`, zero network attack surface |
| **Direct state write when app is offline** | Graceful degradation — `sb switch` works even if the user hasn't launched the app |
| **GCP label convention for secrets** | Avoids embedding structure in secret names; labels are first-class GCP metadata |
| **`LSUIElement = YES`** | App is an agent (no Dock icon); lives exclusively in the menu bar |
| **Go module path `github.com/marcusguttenplan/sb`** | Matches the open-source repo; ready for extraction if CLI is split |

---

*Last updated: June 2026. Architecture reflects the current codebase; see the Packmule plan for the forward-looking roadmap.*
