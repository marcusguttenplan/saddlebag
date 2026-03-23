import Foundation

/// Manages persistent user preferences (labels, tags, favorites)
/// Stored as JSON at ~/.saddlebag/config.json
public actor UserConfigService {
    private let configPath: String
    private var config: UserConfig

    public init() {
        let saddlebagDir = "\(NSHomeDirectory())/.saddlebag"
        self.configPath = "\(saddlebagDir)/config.json"

        // Load existing config or use defaults
        if let data = FileManager.default.contents(atPath: configPath),
           let loaded = try? JSONDecoder().decode(UserConfig.self, from: data) {
            self.config = loaded
        } else {
            self.config = .default
        }
    }

    /// Get the current config
    public func getConfig() -> UserConfig {
        config
    }

    /// Set a human-readable label for a profile
    public func setLabel(for profileName: String, label: String?) throws {
        if let label, !label.isEmpty {
            config.profileLabels[profileName] = label
        } else {
            config.profileLabels.removeValue(forKey: profileName)
        }
        try save()
    }

    /// Set the environment tag for a profile
    public func setTag(for profileName: String, tag: ProfileTag?) throws {
        if let tag {
            config.profileTags[profileName] = tag
        } else {
            config.profileTags.removeValue(forKey: profileName)
        }
        try save()
    }

    /// Toggle a profile as a favorite
    public func toggleFavorite(_ profileName: String) throws {
        if config.favorites.contains(profileName) {
            config.favorites.removeAll { $0 == profileName }
        } else {
            config.favorites.append(profileName)
        }
        try save()
    }

    /// Set the active AWS profile
    public func setActiveProfile(_ profileName: String?) throws {
        config.activeAWSProfile = profileName
        try save()
    }

    /// Set the refresh interval
    public func setRefreshInterval(_ seconds: Int) throws {
        config.refreshInterval = max(10, seconds)
        try save()
    }

    /// Update menubar display settings
    public func setMenuBarDisplay(aws: Bool, time: Bool, gcp: Bool, desk: Bool) throws {
        config.showAWSAccountInMenuBar = aws
        config.showTimeRemainingInMenuBar = time
        config.showGCPProjectInMenuBar = gcp
        config.showDeskInMenuBar = desk
        try save()
    }

    /// Toggle screenshot mode for data obfuscation
    public func setScreenshotMode(_ enabled: Bool) throws {
        config.screenshotMode = enabled
        try save()
    }

    /// Add a custom tag
    public func addCustomTag(name: String) throws {
        guard !name.isEmpty, !config.customTags.contains(where: { $0.name == name }) else { return }
        config.customTags.append(CustomTag(name: name))
        try save()
    }

    /// Remove a custom tag and clear it from all profiles using it
    public func removeCustomTag(name: String) throws {
        config.customTags.removeAll { $0.name == name }
        // Remove tag assignments using this custom tag
        let customTag = ProfileTag.custom(name)
        config.profileTags = config.profileTags.filter { $0.value != customTag }
        try save()
    }

    // MARK: - Persistence

    private func save() throws {
        do {
            let dir = (configPath as NSString).deletingLastPathComponent
            try FileManager.default.createDirectory(
                atPath: dir,
                withIntermediateDirectories: true
            )

            let encoder = JSONEncoder()
            encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
            let data = try encoder.encode(config)
            try data.write(to: URL(fileURLWithPath: configPath))
        } catch {
            throw SaddlebagError.fileWriteError(path: configPath, reason: error.localizedDescription)
        }
    }
}
