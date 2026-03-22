import Foundation
import SwiftUI

/// Built-in environment tags for color-coding profiles
public enum BuiltInTag: String, Codable, CaseIterable, Sendable {
    case prod
    case staging
    case dev
    case personal

    public var label: String {
        switch self {
        case .prod: return "Production"
        case .staging: return "Staging"
        case .dev: return "Development"
        case .personal: return "Personal"
        }
    }

    public var color: Color {
        switch self {
        case .prod: return .red
        case .staging: return .orange
        case .dev: return .green
        case .personal: return .blue
        }
    }

    public var sortOrder: Int {
        switch self {
        case .prod: return 0
        case .staging: return 1
        case .dev: return 2
        case .personal: return 3
        }
    }
}

/// A profile tag — either a built-in environment tag or a user-created custom tag
public enum ProfileTag: Codable, Hashable, Sendable {
    case builtIn(BuiltInTag)
    case custom(String)

    public var label: String {
        switch self {
        case .builtIn(let tag): return tag.label
        case .custom(let name): return name
        }
    }

    public var color: Color {
        switch self {
        case .builtIn(let tag): return tag.color
        case .custom: return .purple
        }
    }

    public var emoji: String {
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

    public var sortOrder: Int {
        switch self {
        case .builtIn(let tag): return tag.sortOrder
        case .custom: return 10 // custom tags sort after built-in
        }
    }

    /// All built-in tags for pickers
    public static var builtInCases: [ProfileTag] {
        BuiltInTag.allCases.map { .builtIn($0) }
    }

    // MARK: - Codable (backward compatible with old "prod"/"staging" strings)

    public init(from decoder: Decoder) throws {
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

    public func encode(to encoder: Encoder) throws {
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
public struct CustomTag: Codable, Hashable, Sendable, Identifiable {
    public let name: String
    public var id: String { name }

    public init(name: String) {
        self.name = name
    }
}

/// Persisted user preferences
public struct UserConfig: Codable, Sendable {
    /// Map profile name → human-readable label
    /// e.g. "cc-dns-admin" → "DNS Admin"
    public var profileLabels: [String: String]

    /// Map profile name → environment tag
    public var profileTags: [String: ProfileTag]

    /// Pinned profile names shown at top of list
    public var favorites: [String]

    /// Seconds between token status refreshes
    public var refreshInterval: Int

    /// Currently active AWS profile name
    public var activeAWSProfile: String?

    // Menubar display settings
    public var showAWSAccountInMenuBar: Bool
    public var showTimeRemainingInMenuBar: Bool
    public var showGCPProjectInMenuBar: Bool

    /// User-created custom tags
    public var customTags: [CustomTag]

    /// Whether to redact sensitive data for screenshots
    public var screenshotMode: Bool

    enum CodingKeys: String, CodingKey {
        case profileLabels, profileTags, favorites, refreshInterval
        case activeAWSProfile, showAWSAccountInMenuBar
        case showTimeRemainingInMenuBar, showGCPProjectInMenuBar
        case customTags, screenshotMode
    }

    public init(from decoder: Decoder) throws {
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
        screenshotMode = try container.decodeIfPresent(Bool.self, forKey: .screenshotMode) ?? false
    }

    public init(
        profileLabels: [String: String],
        profileTags: [String: ProfileTag],
        favorites: [String],
        refreshInterval: Int,
        activeAWSProfile: String?,
        showAWSAccountInMenuBar: Bool,
        showTimeRemainingInMenuBar: Bool,
        showGCPProjectInMenuBar: Bool,
        customTags: [CustomTag],
        screenshotMode: Bool = false
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
        self.screenshotMode = screenshotMode
    }

    public static let `default` = UserConfig(
        profileLabels: [:],
        profileTags: [:],
        favorites: [],
        refreshInterval: 60,
        activeAWSProfile: nil,
        showAWSAccountInMenuBar: false,
        showTimeRemainingInMenuBar: false,
        showGCPProjectInMenuBar: false,
        customTags: [],
        screenshotMode: false
    )

    /// Get the display label for a profile, falling back to the profile name
    public func displayLabel(for profileName: String) -> String {
        profileLabels[profileName] ?? profileName
    }

    /// Check if a profile is favorited
    public func isFavorite(_ profileName: String) -> Bool {
        favorites.contains(profileName)
    }

    /// All available tags: built-in + custom
    public var allTags: [ProfileTag] {
        ProfileTag.builtInCases + customTags.map { .custom($0.name) }
    }
}
