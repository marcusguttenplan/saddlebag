# Saddlebag

A lightweight macOS menu bar app for managing multi-cloud credentials across **AWS** and **GCP** — all from one place.

## What It Does

Saddlebag lives in your menu bar and gives you a unified view of your cloud accounts:

- **AWS** — reads your `~/.aws/config` profiles, tracks SSO sessions and token expiry
- **GCP** — reads `gcloud` configurations, shows active accounts and projects
- **Quick switching** — swap active profiles and configurations without touching the terminal
- **Auth flows** — trigger `gcloud auth login` and `gcloud auth application-default login` directly from the UI
- **Settings** — manage project tags, labels, and per-account preferences
- **Screenshot Mode** — obfuscate account IDs, emails, SSO URLs, and project IDs for safe screen sharing

## Requirements

- macOS 15.0+
- Swift 6.0+
- AWS CLI and/or `gcloud` CLI installed (for credential sources)

## Build & Install

Saddlebag uses **Swift Package Manager** and ships with a `Makefile` for convenience.

```bash
# Build release binary
make build

# Build + create Saddlebag.app bundle
make bundle

# Build + bundle + copy to /Applications
make install

# Build + install + launch
make run

# Remove build artifacts
make clean
```

## Project Structure

```
Saddlebag/
├── SaddlebagApp.swift          # App entry point (MenuBarExtra)
├── Info.plist                  # App bundle metadata
├── Models/
│   ├── AWSProfile.swift        # AWS profile model
│   ├── GCPConfiguration.swift  # GCP configuration model
│   ├── SSOSession.swift        # AWS SSO session model
│   ├── SSOTokenCache.swift     # SSO token cache model
│   └── UserConfig.swift        # User preferences
├── Services/
│   ├── AWSConfigService.swift  # Parses ~/.aws/config
│   ├── GCPConfigService.swift  # Reads gcloud configurations
│   ├── Obfuscator.swift        # Data redaction for screenshot mode
│   ├── SSOSessionService.swift # Manages SSO token lifecycle
│   ├── ShellService.swift      # Shell command execution
│   └── UserConfigService.swift # User config persistence
├── ViewModels/
│   └── AccountsViewModel.swift # Main view model
└── Views/
    ├── MenuBarView.swift       # Menu bar dropdown UI
    ├── SettingsView.swift       # Settings window UI
    ├── SettingsWindowManager.swift
    └── Components/
        ├── HorseshoeIcon.swift  # Custom app icon
        └── ProfileRowView.swift # Reusable profile row
```

## Screenshot Mode

Toggle **Settings → General → Obfuscate sensitive data** to redact sensitive information across the entire UI. This lets you safely take screenshots or record demos without exposing real credentials.

| Data Type | Example | Redacted |
|-----------|---------|----------|
| Email | `user@company.com` | `u•••@c•••.com` |
| Account ID | `123456789012` | `••••••••9012` |
| SSO URL | `https://acme.awsapps.com/start` | `https://••••.awsapps.com/start` |
| Project ID | `my-project-prod-1234` | `••••-••••-1234` |
| Profile/Portal names | `courseclear` | `c••••••••••r` |

## License

Private — not currently licensed for distribution.
