# Saddlebag Widgets — Implementation Plan

## Overview

Build 4 macOS WidgetKit widgets for Notification Center and desktop. Same code powers both placements — WidgetKit handles it automatically on macOS 14+.

> [!IMPORTANT]
> Widgets **must** live in a Widget Extension target. Pure SwiftPM can't define extension targets, so this requires an Xcode project wrapper.

---

## Project Restructure

### Current: Pure SwiftPM
```
2026_test-app/
├── Package.swift          ← single executable target
├── Saddlebag/             ← all source code
└── Tests/
```

### New: Xcode Project + SwiftPM Shared Package
```
2026_test-app/
├── Saddlebag.xcodeproj          ← new Xcode project
├── SaddlebagShared/             ← new Swift package (shared models)
│   ├── Package.swift
│   └── Sources/
│       ├── UserConfig.swift         ← moved from Saddlebag/Models
│       ├── SSOTokenCache.swift      ← moved
│       ├── AWSProfile.swift         ← moved
│       ├── GCPConfiguration.swift   ← moved
│       ├── SSOSession.swift         ← moved
│       └── WidgetData.swift         ← new: lightweight snapshot for widgets
├── Saddlebag/                   ← main app target (mostly unchanged)
│   ├── SaddlebagApp.swift
│   ├── Info.plist
│   ├── Services/
│   ├── ViewModels/
│   └── Views/
├── SaddlebagWidgets/            ← new widget extension target
│   ├── SaddlebagWidgets.swift       ← WidgetBundle entry point
│   ├── Providers/
│   │   ├── AccountStatusProvider.swift
│   │   ├── TokenCountdownProvider.swift
│   │   ├── QuickSwitcherProvider.swift
│   │   └── MultiCloudProvider.swift
│   ├── Views/
│   │   ├── AccountStatusWidget.swift
│   │   ├── TokenCountdownWidget.swift
│   │   ├── QuickSwitcherWidget.swift
│   │   └── MultiCloudWidget.swift
│   └── Intents/
│       └── SwitchProfileIntent.swift
├── Package.swift                ← keep for backward compat / CLI build
└── Tests/
```

### Data Sharing: App Groups

The main app writes a lightweight JSON snapshot to a shared container. Widgets read from it.

**App Group ID**: `group.com.saddlebag.app`

```swift
// WidgetData.swift — in SaddlebagShared package
struct WidgetData: Codable {
    let activeAWSProfile: String?
    let activeAWSLabel: String?
    let activeAWSTag: ProfileTag?
    let activeGCPProject: String?
    let activeGCPConfig: String?

    let tokenStatuses: [TokenSnapshot]
    let awsProfiles: [ProfileSnapshot]  // for quick switcher

    let lastUpdated: Date

    struct TokenSnapshot: Codable {
        let portalName: String
        let expiresAt: Date
        let isExpired: Bool
    }

    struct ProfileSnapshot: Codable, Identifiable {
        let name: String
        let label: String
        let tag: ProfileTag?
        let isFavorite: Bool
        var id: String { name }
    }
}
```

The main app writes this on every refresh:
```swift
// In AccountsViewModel.refresh()
let widgetData = WidgetData(...)
let url = FileManager.default
    .containerURL(forSecurityApplicationGroupIdentifier: "group.com.saddlebag.app")!
    .appendingPathComponent("widget-data.json")
try JSONEncoder().encode(widgetData).write(to: url)
WidgetCenter.shared.reloadAllTimelines()
```

---

## Widget 1: Active Account Status

**Sizes**: Small, Medium

| Size | Layout |
|------|--------|
| Small | Active profile name + tag badge + TTL countdown |
| Medium | AWS profile + GCP project side by side, both with tags + TTL |

**Timeline**: Refresh every 5 minutes (timeline entries), plus on-demand when main app writes new data.

**Key views**:
- Profile name (label or raw name)
- Tag color dot (🔴 prod, 🟢 dev, etc.)
- Credential time remaining as a `Text(.date, style: .relative)` for automatic countdown
- Tap opens main app

---

## Widget 2: SSO Token Countdown

**Sizes**: Small, Circular (accessory)

| Size | Layout |
|------|--------|
| Small | Circular gauge showing % time remaining, formatted TTL below |
| Accessory Circular | Compact gauge with time only (for Notification Center) |

**Key feature**: Uses SwiftUI `Gauge` with a circular style and `Text(.date, style: .timer)` for live countdown without timeline refreshes.

```swift
Gauge(value: percentRemaining, in: 0...1) {
    Text(expiresAt, style: .timer)
} currentValueLabel: {
    Text(timeFormatted)
}
.gaugeStyle(.accessoryCircular)
```

**Color logic**:
- Green (> 30 min remaining)
- Orange (5–30 min)
- Red (< 5 min or expired)

---

## Widget 3: Quick Switcher

**Sizes**: Medium, Large

**Interactive**: Uses `AppIntents` for buttons — macOS 14+ supports this natively.

| Size | Layout |
|------|--------|
| Medium | 3-4 favorite profiles as tappable buttons |
| Large | All favorites + recently used, grouped by tag |

**AppIntent for switching**:
```swift
struct SwitchAWSProfileIntent: AppIntent {
    static var title: LocalizedStringResource = "Switch AWS Profile"

    @Parameter(title: "Profile Name")
    var profileName: String

    func perform() async throws -> some IntentResult {
        // Write new active profile to shared container
        // Signal main app via Darwin notification
        // Reload widget timeline
        return .result()
    }
}
```

Each profile button renders with:
- Tag color accent
- Star icon if favorited
- Checkmark on the currently active one

---

## Widget 4: Multi-Cloud Overview

**Sizes**: Medium, Large

| Size | Layout |
|------|--------|
| Medium | 2 columns: AWS status (left), GCP status (right) |
| Large | Full breakdown: all portals with token status + active GCP project |

**Layout (Large)**:
```
┌─────────────────────────────────┐
│ ☁️  Multi-Cloud Status          │
├────────────────┬────────────────┤
│ AWS            │ GCP            │
│ ● prod-admin   │ ● my-project   │
│   7h 42m       │   Active       │
│                │                │
│ Portal Status  │ Account        │
│ ✅ acme.aws... │ ✅ user@...    │
│ ⚠️ other.aws..│                │
└────────────────┴────────────────┘
```

---

## Widget Bundle Entry Point

```swift
// SaddlebagWidgets.swift
import WidgetKit
import SwiftUI

@main
struct SaddlebagWidgets: WidgetBundle {
    var body: some Widget {
        AccountStatusWidget()
        TokenCountdownWidget()
        QuickSwitcherWidget()
        MultiCloudOverviewWidget()
    }
}
```

---

## Implementation Steps

### Phase 0: Project Restructure
1. Create `SaddlebagShared` local Swift package containing all models
2. Create `Saddlebag.xcodeproj` with the main app target importing `SaddlebagShared`
3. Add App Group capability (`group.com.saddlebag.app`)
4. Verify main app still builds and runs
5. Add data writing to shared container on each refresh
6. Update `Makefile` for the new Xcode-based build

### Phase 1: Account Status Widget (start here)
1. Add Widget Extension target in Xcode
2. Create `WidgetData` model in shared package
3. Implement `AccountStatusProvider` (TimelineProvider)
4. Build Small + Medium views
5. Test on desktop and in Notification Center

### Phase 2: Token Countdown Widget
1. Implement `TokenCountdownProvider`
2. Build gauge-based views
3. Use `Text(.date, style: .timer)` for live countdown

### Phase 3: Quick Switcher Widget
1. Create `SwitchAWSProfileIntent` AppIntent
2. Add intent to both targets
3. Build interactive button grid
4. Wire Darwin notifications for cross-process signaling

### Phase 4: Multi-Cloud Overview Widget
1. Implement `MultiCloudProvider`
2. Build 2-column layout
3. Add all portal/account status data to `WidgetData`

---

## Key Considerations

- **No network calls in widgets** — widgets read from the shared JSON file only. The main app is responsible for refreshing data and signaling widget reloads.
- **Desktop + Notification Center** — WidgetKit handles both placements automatically. Same code, same sizes, same views. Users choose where to place them.
- **Interactive widgets require macOS 14+** — already satisfied since the app targets macOS 15.
- **Timeline budgets** — macOS gives generous timeline budgets. A 5-minute refresh interval is fine. The timer display (`Text(.date, style: .timer)`) updates live without using timeline budget.
- **Signing** — for local dev without App Store, ad-hoc signing works. The Makefile will need `xcodebuild` instead of `swift build` once widgets are added.
