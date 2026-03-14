import Foundation
import SwiftUI

/// Built-in environment tags for color-coding profiles
enum BuiltInTag: String, Codable, CaseIterable, Sendable {
    case prod
    case staging
    case dev
    case personal

    var label: String {
        switch self {
        case .prod: return "Production"
        case .staging: return "Staging"
        case .dev: return "Development"
        case .personal: return "Personal"
        }
    }

    var color: Color {
        switch self {
        case .prod: return .red
        case .staging: return .orange
        case .dev: return .green
        case .personal: return .blue
        }
    }

    var sortOrder: Int {
        switch self {
        case .prod: return 0
        case .staging: return 1
        case .dev: return 2
        case .personal: return 3
        }
    }
}

/// A profile tag — either a built-in environment tag or a user-created custom tag
enum ProfileTag: Codable, Hashable, Sendable {
    case builtIn(BuiltInTag)
    case custom(String)

    var label: String {
        switch self {
        case .builtIn(let tag): return tag.label
        case .custom(let name): return name
        }
    }

    var color: Color {
        switch self {
        case .builtIn(let tag): return tag.color
        case .custom: return .purple
        }
    }

    var emoji: String {
        switch self {
        case .builtIn(let tag):
            switch tag {
            case .prod: return "🔴"
            case .staging: return "🟡"
            case .dev: return "🟢"
            case .personal: return "🔵"
            }
        case .custom: return "🟣"
        }
    }

    var sortOrder: Int {
        switch self {
        case .builtIn(let tag): return tag.sortOrder
        case .custom: return 10 // custom tags sort after built-in
        }
    }

    /// All built-in tags for pickers
    static var builtInCases: [ProfileTag] {
        BuiltInTag.allCases.map { .builtIn($0) }
    }

    // MARK: - Codable (backward compatible with old "prod"/"staging" strings)

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        let raw = try container.decode(String.self)

        // Try built-in tag first (backward compatible with old config)
        if let builtIn = BuiltInTag(rawValue: raw) {
            self = .builtIn(builtIn)
        } else if raw.hasPrefix("custom:") {
            self = .custom(String(raw.dropFirst(7)))
        } else {
            // Treat unknown strings as custom tags
            self = .custom(raw)
        }
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .builtIn(let tag):
            try container.encode(tag.rawValue)
        case .custom(let name):
            try container.encode("custom:\(name)")
        }
    }
}

/// A user-defined custom tag stored in config
struct CustomTag: Codable, Hashable, Sendable, Identifiable {
    let name: String
    var id: String { name }
}

/// Persisted user preferences
struct UserConfig: Codable, Sendable {
    /// Map profile name → human-readable label
    /// e.g. "cc-dns-admin" → "DNS Admin"
    var profileLabels: [String: String]

    /// Map profile name → environment tag
    var profileTags: [String: ProfileTag]

    /// Pinned profile names shown at top of list
    var favorites: [String]

    /// Seconds between token status refreshes
    var refreshInterval: Int

    /// Currently active AWS profile name
    var activeAWSProfile: String?

    // Menubar display settings
    var showAWSAccountInMenuBar: Bool
    var showTimeRemainingInMenuBar: Bool
    var showGCPProjectInMenuBar: Bool

    /// User-created custom tags
    var customTags: [CustomTag]

    enum CodingKeys: String, CodingKey {
        case profileLabels, profileTags, favorites, refreshInterval
        case activeAWSProfile, showAWSAccountInMenuBar
        case showTimeRemainingInMenuBar, showGCPProjectInMenuBar
        case customTags
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        profileLabels = try container.decode([String: String].self, forKey: .profileLabels)
        profileTags = try container.decode([String: ProfileTag].self, forKey: .profileTags)
        favorites = try container.decode([String].self, forKey: .favorites)
        refreshInterval = try container.decode(Int.self, forKey: .refreshInterval)
        activeAWSProfile = try container.decodeIfPresent(String.self, forKey: .activeAWSProfile)
        showAWSAccountInMenuBar = try container.decodeIfPresent(Bool.self, forKey: .showAWSAccountInMenuBar) ?? false
        showTimeRemainingInMenuBar = try container.decodeIfPresent(Bool.self, forKey: .showTimeRemainingInMenuBar) ?? false
        showGCPProjectInMenuBar = try container.decodeIfPresent(Bool.self, forKey: .showGCPProjectInMenuBar) ?? false
        customTags = try container.decodeIfPresent([CustomTag].self, forKey: .customTags) ?? []
    }

    init(
        profileLabels: [String: String],
        profileTags: [String: ProfileTag],
        favorites: [String],
        refreshInterval: Int,
        activeAWSProfile: String?,
        showAWSAccountInMenuBar: Bool,
        showTimeRemainingInMenuBar: Bool,
        showGCPProjectInMenuBar: Bool,
        customTags: [CustomTag]
    ) {
        self.profileLabels = profileLabels
        self.profileTags = profileTags
        self.favorites = favorites
        self.refreshInterval = refreshInterval
        self.activeAWSProfile = activeAWSProfile
        self.showAWSAccountInMenuBar = showAWSAccountInMenuBar
        self.showTimeRemainingInMenuBar = showTimeRemainingInMenuBar
        self.showGCPProjectInMenuBar = showGCPProjectInMenuBar
        self.customTags = customTags
    }

    static let `default` = UserConfig(
        profileLabels: [:],
        profileTags: [:],
        favorites: [],
        refreshInterval: 60,
        activeAWSProfile: nil,
        showAWSAccountInMenuBar: false,
        showTimeRemainingInMenuBar: false,
        showGCPProjectInMenuBar: false,
        customTags: []
    )

    /// Get the display label for a profile, falling back to the profile name
    func displayLabel(for profileName: String) -> String {
        profileLabels[profileName] ?? profileName
    }

    /// Check if a profile is favorited
    func isFavorite(_ profileName: String) -> Bool {
        favorites.contains(profileName)
    }

    /// All available tags: built-in + custom
    var allTags: [ProfileTag] {
        ProfileTag.builtInCases + customTags.map { .custom($0.name) }
    }
}
