# Saddlebag

A native **macOS menu bar app** for managing multi-cloud credentials and developer context — all from one place.

## What It Does

Saddlebag lives in your menu bar and gives you a unified view of your cloud accounts:

- **AWS** — reads your `~/.aws/config` profiles, tracks SSO sessions and token expiry
- **GCP** — reads `gcloud` configurations, switch active configs, trigger auth flows
- **Quick switching** — swap active profiles and configurations without touching the terminal
- **Auth flows** — trigger `gcloud auth login` and `gcloud auth application-default login` directly from the UI
- **Settings** — manage project tags, labels, and per-account preferences
- **Screenshot Mode** — obfuscate account IDs, emails, SSO URLs, and project IDs for safe screen sharing

### Roadmap (POC)

- **`sb` CLI** — Go companion CLI for shell integration, replacing the fragile `.zshrc` bridge
- **Desks** — first-class context bundles (AWS profile + GCP config + git identity + SSH key + env vars)
- **Credential health** — background monitoring with macOS notifications (🟢 → 🟡 → 🔴)
- **Git identity guard** — pre-commit hook that blocks commits with wrong `user.email`

## Requirements

- macOS 15.0+
- Xcode 16+ (Swift 6.0)
- AWS CLI and/or `gcloud` CLI installed (for credential sources)

## Architecture

```
_dev/
├── Saddlebag.xcodeproj         # Xcode project
├── SaddlebagShared/            # Shared Swift package (models + services)
│   ├── Package.swift
│   └── Sources/
│       ├── Models/
│       │   ├── AWSProfile.swift
│       │   ├── GCPConfiguration.swift
│       │   ├── SSOSession.swift
│       │   ├── SSOTokenCache.swift
│       │   ├── SaddlebagError.swift
│       │   └── UserConfig.swift
│       └── Services/
│           ├── AWSConfigService.swift
│           ├── GCPConfigService.swift
│           ├── Obfuscator.swift
│           ├── ShellService.swift
│           ├── SSOSessionService.swift
│           └── UserConfigService.swift
├── Saddlebag/                  # Main app target
│   ├── SaddlebagApp.swift
│   ├── Info.plist
│   ├── ViewModels/
│   │   └── AccountsViewModel.swift
│   └── Views/
│       ├── MenuBarView.swift
│       ├── SettingsView.swift
│       ├── SettingsWindowManager.swift
│       └── Components/
│           ├── HorseshoeIcon.swift
│           └── ProfileRowView.swift
└── sb/                         # Go CLI (planned)
    ├── go.mod
    ├── .go-version
    └── cmd/
```

### Key design decisions

- **Xcode project** — enables Widget Extension targets, App Groups, and proper code signing
- **SaddlebagShared** — local Swift package shared between app and future widget targets
- **`~/.saddlebag/`** — all user config lives here (not `~/Library/Application Support/`)
  - `config.json` — user preferences (labels, tags, favorites)
  - `state.json` — live state (active desk, AWS profile, GCP config)
  - `desks/*.toml` — desk definitions (one per context)
- **Go CLI** — `sb` companion binary, communicates with app via Unix socket at `/tmp/saddlebag.sock`

## Build & Run

Open `Saddlebag.xcodeproj` in Xcode and build (⌘B) / run (⌘R).

The app runs as a menu bar agent (`LSUIElement = YES`) — no Dock icon, just the menu bar.

## Screenshot Mode

Toggle **Settings → General → Obfuscate sensitive data** to redact sensitive information across the entire UI.

| Data Type | Example | Redacted |
|-----------|---------|----------|
| Email | `user@company.com` | `u•••@c•••.com` |
| Account ID | `123456789012` | `••••••••9012` |
| SSO URL | `https://acme.awsapps.com/start` | `https://••••.awsapps.com/start` |
| Project ID | `my-project-prod-1234` | `••••-••••-1234` |
| Profile/Portal names | `courseclear` | `c••••••••••r` |

## License

Private — not currently licensed for distribution.
