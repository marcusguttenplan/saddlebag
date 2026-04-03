# Saddlebag

A native **macOS menu bar app** for managing multi-cloud credentials and developer context — all from one place.

Saddlebag manages "Desks" -- synchronized environment bundles that hot-swap AWS/GCP profiles, git identities, and shell variables across your entire system. It ensures your terminal and GUI always match your current project context while redacting sensitive IDs for safe screen sharing.

It also integrates with **GCP Secret Manager**, letting you browse, copy, and generate `.env` files from cloud secrets using a label convention.

## What It Does

Saddlebag lives in your menu bar and gives you a unified view of your cloud accounts:

- **AWS** — reads your `~/.aws/config` profiles, tracks SSO sessions and token expiry
- **GCP** — reads `gcloud` configurations, switch active configs, trigger auth flows
- **Secrets** — browse GCP Secret Manager secrets, copy values, generate `.env` files using label convention
- **Quick switching** — swap active profiles and configurations without touching the terminal
- **Auth flows** — trigger `gcloud auth login` and `gcloud auth application-default login` directly from the UI
- **Settings** — manage project tags, labels, and per-account preferences
- **Screenshot Mode** — obfuscate account IDs, emails, SSO URLs, and project IDs for safe screen sharing

### Roadmap

- **Credential health** — background monitoring with macOS notifications (🟢 → 🟡 → 🔴)
- **Git identity guard** — pre-commit hook that blocks commits with wrong `user.email`
- **AWS Secrets Manager** — extend secrets integration to AWS

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
│       │   ├── Secret.swift
│       │   ├── SSOSession.swift
│       │   ├── SSOTokenCache.swift
│       │   ├── SaddlebagError.swift
│       │   └── UserConfig.swift
│       └── Services/
│           ├── AWSConfigService.swift
│           ├── GCPConfigService.swift
│           ├── Obfuscator.swift
│           ├── SecretManagerService.swift
│           ├── ShellService.swift
│           ├── SSOSessionService.swift
│           └── UserConfigService.swift
├── Saddlebag/                  # Main app target
│   ├── SaddlebagApp.swift
│   ├── Info.plist
│   ├── ViewModels/
│   │   ├── AccountsViewModel.swift
│   │   └── SecretsViewModel.swift
│   └── Views/
│       ├── MainAppView.swift
│       ├── MenuBarView.swift
│       ├── SecretsView.swift
│       ├── SettingsView.swift
│       ├── SettingsWindowManager.swift
│       └── Components/
│           ├── HorseshoeIcon.swift
│           └── ProfileRowView.swift
└── sb/                         # Go CLI companion
    ├── go.mod
    ├── .go-version
    ├── cmd/
    │   ├── env.go
    │   ├── root.go
    │   └── secrets.go
    └── internal/
        ├── config/
        ├── desk/
        ├── secrets/
        │   └── secrets.go
        └── state/
```

### Key design decisions

- **Xcode project** — enables Widget Extension targets, App Groups, and proper code signing
- **SaddlebagShared** — local Swift package shared between app and future widget targets
- **`~/.saddlebag/`** — all user config lives here (not `~/Library/Application Support/`)
  - `config.json` — user preferences (labels, tags, favorites)
  - `state.json` — live state (active desk, AWS profile, GCP config)
  - `desks/*.toml` — desk definitions (one per context)
- **Go CLI** — `sb` companion binary, communicates with app via Unix socket at `/tmp/saddlebag.sock`
- **Secrets label convention** — secrets use GCP labels (`org`, `service`, `stage`, `var`) to drive env file generation

## Secrets Manager

Saddlebag integrates with GCP Secret Manager. Secrets are organized using a **label convention**:

| Label | Example | Purpose |
|-------|---------|--------|
| `org` | `courseclear` | Organization/client grouping |
| `service` | `api`, `web` | Becomes the directory in env output |
| `stage` | `dev`, `prod` | Becomes the file suffix `.env.$stage` |

The **secret name** is used as the env var name directly. A secret named `DATABASE_URL` with labels `service=api, stage=prod` generates:

```
# api/.env.prod
DATABASE_URL="<secret_value>"
```

> [!NOTE]
> GCP labels must be lowercase. Saddlebag automatically lowercases label values.

### App Features

- **Secrets tab** in the main window — browse, filter by labels, multi-select, generate `.env` files
- **Menu bar context menu** — right-click any GCP project → Secrets → click to copy a value
- **Clipboard options** — copy single value, copy as `.env` block (grouped by service/stage), copy as JSON

### CLI (`sb secrets`)

```bash
sb secrets list [--project=X] [--org=cc] [--service=api] [--stage=prod]
sb secrets get SECRET_NAME [--project=X]
sb secrets create SECRET_NAME --value=... --org=cc --service=api --stage=prod
sb secrets tag SECRET_NAME --org=cc --service=api --stage=prod
sb secrets env [--project=X] [--service=api] [--stage=prod] [--output=./]
sb secrets copy SECRET_NAME [--project=X]
```

- `--project` is optional — inferred from active desk → GCP config → aliases in `config.json`
- `create` upserts — if the secret exists, it updates labels and adds a new version
- `tag` updates labels on existing secrets (additive, preserves existing labels)
- All commands accept `--provider=gcp` (default)

## Build & Run

Open `Saddlebag.xcodeproj` in Xcode and build (⌘B) / run (⌘R).

The app runs as a menu bar agent (`LSUIElement = YES`) — no Dock icon, just the menu bar.

## Releases (distribution)

**Saddlebag.app** — Archive in Xcode with **Direct Distribution**, notarize, export, then staple and zip (or DMG) the app. For each version, create a **GitHub Release** tagged `v1.2.3` and attach the notarized archive (e.g. `Saddlebag.zip`).

**`sb` CLI** — Build on a Mac, **Developer ID** sign, **notarize**, **staple**, zip, then upload **`sb-darwin-arm64.zip`**, **`sb-darwin-amd64.zip`**, and **`checksums-sha256.txt`** to the same GitHub Release (no CI required). See **Manual release: `sb`** below.

### Manual release: `sb`

From repo root (`_dev/`), run `mkdir -p dist`. Use **Developer ID Application** and **`notarytool`** (same flow as Saddlebag). Point `notarytool` at your **API key** (or Apple ID) as you already do for the app.

```bash
TAG=v1.0.1
mkdir -p dist
cd sb
for arch in arm64 amd64; do
  bin="sb-darwin-${arch}"
  GOOS=darwin GOARCH=$arch go build -trimpath \
    -ldflags "-s -w -X github.com/marcusguttenplan/sb/cmd.version=${TAG}" \
    -o "../dist/${bin}" .
done
cd ../dist
IDENTITY='Developer ID Application: Marcus Guttenplan (WLBDHVYSL6)'   # from Keychain / security find-identity

for arch in arm64 amd64; do
  bin="sb-darwin-${arch}"
  zip="${bin}.zip"
  codesign --force --timestamp --options runtime --sign "$IDENTITY" "$bin"
  zip -q -y "$zip" "$bin"
  xcrun notarytool submit "$zip" --wait --key /path/to/AuthKey_XXX.p8 --key-id XXX --issuer YOUR_ISSUER_UUID
  xcrun stapler staple "$zip"
done
shasum -a 256 sb-darwin-*.zip > checksums-sha256.txt
```

Upload with **GitHub Release UI** or:

```bash
gh release upload "$TAG" sb-darwin-arm64.zip sb-darwin-amd64.zip checksums-sha256.txt --clobber
```

Create the release/tag first if needed: `gh release create "$TAG" --title "$TAG" --notes ""`.

### Install on another Mac (script)

Use [`scripts/install-release.sh`](scripts/install-release.sh). **Private repo:** authenticate first (`gh auth login` or a token with **Contents: Read**).

```bash
# From a clone (recommended; repo root is this _dev folder)
cd /path/to/saddlebag
gh auth login
./scripts/install-release.sh v1.0.0

# Per-user CLI install (no sudo for sb binary)
./scripts/install-release.sh v1.0.0 --user

# Latest release (still needs auth for private repo)
./scripts/install-release.sh
```

The script downloads **`sb`** for the machine’s arch (with checksum verification), installs **`Saddlebag.app`** into `/Applications`, and needs **`curl`**, **`python3`**, and **`unzip`**.

**One-liner from GitHub raw** (token required for private repos — do not paste the token into shell history on shared machines):

```bash
export GITHUB_TOKEN=ghp_xxx   # classic PAT with repo scope, or fine-grained with Contents read
curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" \
  https://raw.githubusercontent.com/marcusguttenplan/saddlebag/v1.0.0/scripts/install-release.sh | bash -s -- v1.0.0
```

Manual install without the script: download `sb-darwin-ARCH.zip` and `checksums-sha256.txt` from [Releases](https://github.com/marcusguttenplan/saddlebag/releases), `grep` your zip filename in the checksum file and pipe that line to `shasum -a 256 -c`, unzip the archive, move the `sb-darwin-*` binary to your `PATH` as `sb`, and unzip `Saddlebag.zip` into `/Applications`.

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
