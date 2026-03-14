# Saddlebag Desktop App Expansion — Architecture Plan

## Current State

Saddlebag is a **menu bar-only** SwiftPM app (`LSUIElement = true`) with:
- **5 Services** — `ShellService` (actor), `AWSConfigService`, `GCPConfigService`, `SSOSessionService`, `UserConfigService`
- **1 ViewModel** — `AccountsViewModel` (~380 lines, handles everything)
- **3 Views** — `MenuBarView`, `SettingsView`, `SettingsWindowManager`
- All cloud interactions wrap CLI tools (`aws`, `gcloud`) via `ShellService`

## What Needs to Change

You're going from a **read/display** tool to an **operational** tool. That's a fundamentally different kind of app. Here's how to approach it.

---

## Phase 1: Structural Refactoring (do this first)

Before adding features, fix the architecture so it can support them.

### 1.1 Split the monolithic ViewModel

`AccountsViewModel` currently owns all state and all actions. As you add Tailscale, SSH, and secrets features, this will become unmanageable.

```
ViewModels/
├── AccountsViewModel.swift      ← keeps AWS/GCP account state only
├── TailscaleViewModel.swift     ← new: Tailscale devices, status
├── SSHViewModel.swift           ← new: SSH key mgmt, sshd config
├── SecretsViewModel.swift       ← new: GCP Secret Manager
└── AppState.swift               ← new: shared state container
```

`AppState` is an `@Observable` object that holds shared references and cross-cutting state (e.g., which GCP project is active, which Tailscale devices are reachable).

### 1.2 Add a main window alongside MenuBarExtra

Currently `LSUIElement = true` hides the app from the Dock. You'll want to keep the menu bar utility **and** add a proper window for the more complex features.

```swift
// SaddlebagApp.swift
var body: some Scene {
    // Keep existing menu bar
    MenuBarExtra { ... }

    // Add main window for full features
    Window("Saddlebag", id: "main") {
        ContentView()
    }

    // Keep settings
    Settings { SettingsView() }
}
```

The menu bar stays as the quick-access surface. The main window is where SSH, file transfer, and secrets live.

### 1.3 Introduce a protocol for services

Your current services are concrete classes. As you add more (Tailscale, SSH, SecretManager), consider a light protocol so you can mock and test:

```swift
protocol CloudService: Actor {
    func refresh() async throws
}
```

---

## Phase 2: Feature Priority & Implementation Order

> [!IMPORTANT]
> I'd strongly recommend building these **in this order** — each one builds on the last.

### Feature 1: GCP Secrets Retrieval ⭐ (start here)

**Why first**: You already have GCP auth working. This is the shortest path to new value.

**What it does**: Pull secrets from GCP Secret Manager for local dev (think `.env` generation).

**New files**:
| File | Purpose |
|------|---------|
| `Services/SecretManagerService.swift` | Wraps `gcloud secrets versions access` and `gcloud secrets list` |
| `Models/Secret.swift` | Secret name, version, project association |
| `ViewModels/SecretsViewModel.swift` | Manages secret list, fetch, and export |
| `Views/SecretsView.swift` | Main window tab for browsing and pulling secrets |

**Key CLI commands**:
```bash
gcloud secrets list --project=PROJECT_ID --format=json
gcloud secrets versions access latest --secret=SECRET_NAME --project=PROJECT_ID
```

**UX**: Select a GCP project → see its secrets → copy individual values or export as `.env` file.

---

### Feature 2: Tailscale Device Discovery

**Why second**: Foundation for SSH and file transfer features.

**New files**:
| File | Purpose |
|------|---------|
| `Services/TailscaleService.swift` | Wraps `tailscale status --json` and `tailscale ip` |
| `Models/TailscaleDevice.swift` | Device name, IP, OS, online status, tags |
| `ViewModels/TailscaleViewModel.swift` | Device list, status polling |
| `Views/TailscaleView.swift` | Device grid/list in main window |

**Key CLI commands**:
```bash
tailscale status --json       # all devices, IPs, online status
tailscale ip -4               # local node's Tailscale IP
tailscale ping DEVICE         # check reachability
```

---

### Feature 3: SSH Key Distribution over Tailscale

**Why third**: Requires Tailscale device discovery from Feature 2.

**Two approaches** — choose based on your needs:

#### Option A: SSH CA (certificate-based)
- Generate a CA keypair, sign user keys, distribute CA public key to hosts
- More secure, supports expiry, no individual key management
- Complexity: **High** — requires `ssh-keygen -s` for signing, host-side `TrustedUserCAKeys` config

#### Option B: Key push (simpler)
- `ssh-copy-id` equivalent over Tailscale
- Push `~/.ssh/id_ed25519.pub` to selected devices via `ssh user@TAILSCALE_IP`
- Complexity: **Medium** — straightforward but requires existing SSH access for bootstrap

**New files**:
| File | Purpose |
|------|---------|
| `Services/SSHKeyService.swift` | Key generation, signing (CA), or key push |
| `Models/SSHKey.swift` | Key type, path, fingerprint, associated devices |
| `ViewModels/SSHViewModel.swift` | Key management state |
| `Views/SSHKeysView.swift` | UI for key generation and distribution |

---

### Feature 4: sshd Config Generator

**Why fourth**: Natural companion to SSH key distribution.

**What it does**: Generate sshd_config snippets that restrict SSH access to Tailscale interfaces only.

**Key config patterns**:
```
# Restrict to Tailscale interface only
ListenAddress 100.x.y.z        # Tailscale IP
PasswordAuthentication no
PubkeyAuthentication yes
TrustedUserCAKeys /etc/ssh/ca.pub   # if using CA approach
```

**New files**:
| File | Purpose |
|------|---------|
| `Services/SSHDConfigService.swift` | Template generation, config parsing |
| `Views/SSHDConfigView.swift` | Preview + copy/export generated config |

**UX**: Select a device → generate config → copy to clipboard or push via SSH.

---

### Feature 5: File Transfer Between Tailscale Devices

**Why last**: Most complex, benefits from all prior infrastructure.

**Implementation options**:
1. **`scp`/`rsync` over Tailscale** — simplest, uses SSH infrastructure from Feature 3
2. **`tailscale file` (Taildrop)** — uses Tailscale's built-in file transfer (`tailscale file cp`)
3. **Both** — Taildrop for quick sends, scp/rsync for directory sync

**New files**:
| File | Purpose |
|------|---------|
| `Services/FileTransferService.swift` | Wraps `tailscale file cp` and/or `scp` |
| `ViewModels/FileTransferViewModel.swift` | Transfer queue, progress tracking |
| `Views/FileTransferView.swift` | Drag-and-drop UI, transfer progress |

---

## Proposed Directory Structure (end state)

```
Saddlebag/
├── SaddlebagApp.swift
├── Info.plist
├── Models/
│   ├── AWSProfile.swift
│   ├── GCPConfiguration.swift
│   ├── SSOSession.swift
│   ├── SSOTokenCache.swift
│   ├── UserConfig.swift
│   ├── Secret.swift                 ← new
│   ├── TailscaleDevice.swift        ← new
│   └── SSHKey.swift                 ← new
├── Services/
│   ├── ShellService.swift
│   ├── AWSConfigService.swift
│   ├── GCPConfigService.swift
│   ├── SSOSessionService.swift
│   ├── UserConfigService.swift
│   ├── SecretManagerService.swift   ← new
│   ├── TailscaleService.swift       ← new
│   ├── SSHKeyService.swift          ← new
│   ├── SSHDConfigService.swift      ← new
│   └── FileTransferService.swift    ← new
├── ViewModels/
│   ├── AccountsViewModel.swift
│   ├── AppState.swift               ← new
│   ├── SecretsViewModel.swift       ← new
│   ├── TailscaleViewModel.swift     ← new
│   ├── SSHViewModel.swift           ← new
│   └── FileTransferViewModel.swift  ← new
└── Views/
    ├── MenuBarView.swift
    ├── SettingsView.swift
    ├── SettingsWindowManager.swift
    ├── ContentView.swift            ← new (main window)
    ├── SecretsView.swift            ← new
    ├── TailscaleView.swift          ← new
    ├── SSHKeysView.swift            ← new
    ├── SSHDConfigView.swift         ← new
    ├── FileTransferView.swift       ← new
    └── Components/
        └── HorseshoeIcon.swift
```

## Where to Start

> [!TIP]
> **Immediate next step**: Build `SecretManagerService` + `SecretsView`. You already have working GCP auth, `ShellService`, and the pattern for wrapping CLI commands. You can ship this in a day.

1. Create `SecretManagerService.swift` following the pattern from `GCPConfigService`
2. Create `SecretsViewModel.swift` following the pattern from `AccountsViewModel`
3. Add a `Window` scene to `SaddlebagApp.swift` with a `ContentView` that uses tabs
4. Build the `SecretsView` tab

After that, move to Tailscale device discovery — it unlocks SSH and file transfer.
