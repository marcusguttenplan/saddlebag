import Foundation

/// Manages persistent user preferences (labels, tags, favorites)
/// Stored as JSON at ~/Library/Application Support/Saddlebag/config.json
actor UserConfigService {
    private let configPath: String
    private var config: UserConfig

    init() {
        let appSupport = FileManager.default.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first!.appendingPathComponent("Saddlebag")

        self.configPath = appSupport.appendingPathComponent("config.json").path

        // Load existing config or use defaults
        if let data = FileManager.default.contents(atPath: configPath),
           let loaded = try? JSONDecoder().decode(UserConfig.self, from: data) {
            self.config = loaded
        } else {
            self.config = .default
        }
    }

    /// Get the current config
    func getConfig() -> UserConfig {
        config
    }

    /// Set a human-readable label for a profile
    func setLabel(for profileName: String, label: String?) {
        if let label, !label.isEmpty {
            config.profileLabels[profileName] = label
        } else {
            config.profileLabels.removeValue(forKey: profileName)
        }
        save()
    }

    /// Set the environment tag for a profile
    func setTag(for profileName: String, tag: ProfileTag?) {
        if let tag {
            config.profileTags[profileName] = tag
        } else {
            config.profileTags.removeValue(forKey: profileName)
        }
        save()
    }

    /// Toggle a profile as a favorite
    func toggleFavorite(_ profileName: String) {
        if config.favorites.contains(profileName) {
            config.favorites.removeAll { $0 == profileName }
        } else {
            config.favorites.append(profileName)
        }
        save()
    }

    /// Set the active AWS profile
    func setActiveProfile(_ profileName: String?) {
        config.activeAWSProfile = profileName
        save()
    }

    /// Set the refresh interval
    func setRefreshInterval(_ seconds: Int) {
        config.refreshInterval = max(10, seconds)
        save()
    }

    /// Update menubar display settings
    func setMenuBarDisplay(aws: Bool, time: Bool, gcp: Bool) {
        config.showAWSAccountInMenuBar = aws
        config.showTimeRemainingInMenuBar = time
        config.showGCPProjectInMenuBar = gcp
        save()
    }

    /// Toggle screenshot mode for data obfuscation
    func setScreenshotMode(_ enabled: Bool) {
        config.screenshotMode = enabled
        save()
    }

    /// Add a custom tag
    func addCustomTag(name: String) {
        guard !name.isEmpty, !config.customTags.contains(where: { $0.name == name }) else { return }
        config.customTags.append(CustomTag(name: name))
        save()
    }

    /// Remove a custom tag and clear it from all profiles using it
    func removeCustomTag(name: String) {
        config.customTags.removeAll { $0.name == name }
        // Remove tag assignments using this custom tag
        let customTag = ProfileTag.custom(name)
        config.profileTags = config.profileTags.filter { $0.value != customTag }
        save()
    }

    // MARK: - Persistence

    private func save() {
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
            print("Failed to save user config: \(error)")
        }
    }
}
