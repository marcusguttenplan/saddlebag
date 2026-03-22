import Foundation

/// Parses ~/.aws/config into AWSProfile and SSOSession models
public struct AWSConfigService: Sendable {
    private let configPath: String

    public init(configPath: String? = nil) {
        self.configPath = configPath ?? "\(NSHomeDirectory())/.aws/config"
    }

    /// Load all profiles and SSO sessions from the AWS config file
    public func loadProfiles() throws -> (profiles: [AWSProfile], sessions: [SSOSession]) {
        guard FileManager.default.fileExists(atPath: configPath) else {
            return ([], [])
        }
        let content = try String(contentsOfFile: configPath, encoding: .utf8)
        let sections = parseINI(content)

        var profiles: [AWSProfile] = []
        var sessions: [SSOSession] = []

        for (sectionName, properties) in sections {
            if sectionName.hasPrefix("sso-session ") {
                let sessionName = String(sectionName.dropFirst("sso-session ".count))
                if let startUrl = properties["sso_start_url"] {
                    sessions.append(SSOSession(
                        name: sessionName,
                        startUrl: startUrl,
                        region: properties["sso_region"] ?? "us-east-1",
                        registrationScopes: properties["sso_registration_scopes"] ?? "sso:account:access"
                    ))
                }
            } else {
                // Profile section: either [default] or [profile name]
                let profileName: String
                if sectionName == "default" {
                    profileName = "default"
                } else if sectionName.hasPrefix("profile ") {
                    profileName = String(sectionName.dropFirst("profile ".count))
                } else {
                    continue
                }

                // Only include SSO profiles (have sso_session or sso_account_id)
                guard let ssoSession = properties["sso_session"],
                      let accountId = properties["sso_account_id"],
                      let roleName = properties["sso_role_name"] else {
                    continue
                }

                profiles.append(AWSProfile(
                    name: profileName,
                    ssoSessionName: ssoSession,
                    accountId: accountId,
                    roleName: roleName,
                    region: properties["region"] ?? "us-east-1",
                    output: properties["output"] ?? "json"
                ))
            }
        }

        return (profiles, sessions)
    }

    /// Group profiles by their SSO portal domain
    public func groupedProfiles(
        profiles: [AWSProfile],
        sessions: [SSOSession]
    ) -> [(portal: String, portalDisplayName: String, profiles: [AWSProfile])] {
        let sessionMap = Dictionary(uniqueKeysWithValues: sessions.map { ($0.name, $0) })

        var groups: [String: (displayName: String, profiles: [AWSProfile])] = [:]

        for profile in profiles {
            let session = sessionMap[profile.ssoSessionName]
            let portalDomain = session?.portalDomain ?? "unknown"
            let displayName = session?.portalDisplayName ?? portalDomain

            if groups[portalDomain] == nil {
                groups[portalDomain] = (displayName: displayName, profiles: [])
            }
            groups[portalDomain]?.profiles.append(profile)
        }

        return groups
            .sorted { $0.key < $1.key }
            .map { (portal: $0.key, portalDisplayName: $0.value.displayName, profiles: $0.value.profiles) }
    }

    /// Append a new SSO profile section to ~/.aws/config
    public func appendProfile(name: String, ssoSession: String, accountId: String, roleName: String, region: String) throws {
        let section = """

        [profile \(name)]
        sso_session = \(ssoSession)
        sso_account_id = \(accountId)
        sso_role_name = \(roleName)
        region = \(region)
        output = json
        """

        do {
            let handle = try FileHandle(forWritingTo: URL(fileURLWithPath: configPath))
            handle.seekToEndOfFile()
            handle.write(("\n" + section + "\n").data(using: .utf8)!)
            handle.closeFile()
        } catch {
            throw SaddlebagError.fileWriteError(path: configPath, reason: error.localizedDescription)
        }
    }

    // MARK: - INI Parser

    /// Parse INI-style config into sections with key-value pairs
    private func parseINI(_ content: String) -> [(String, [String: String])] {
        var sections: [(String, [String: String])] = []
        var currentSection: String?
        var currentProperties: [String: String] = [:]

        for line in content.components(separatedBy: .newlines) {
            let trimmed = line.trimmingCharacters(in: .whitespaces)

            // Skip empty lines and comments
            if trimmed.isEmpty || trimmed.hasPrefix("#") || trimmed.hasPrefix(";") {
                continue
            }

            // Section header: [section name]
            if trimmed.hasPrefix("[") && trimmed.hasSuffix("]") {
                // Save previous section
                if let section = currentSection {
                    sections.append((section, currentProperties))
                }

                currentSection = String(trimmed.dropFirst().dropLast())
                currentProperties = [:]
                continue
            }

            // Key-value pair: key = value
            if let equalIndex = trimmed.firstIndex(of: "=") {
                let key = trimmed[trimmed.startIndex..<equalIndex]
                    .trimmingCharacters(in: .whitespaces)
                let value = trimmed[trimmed.index(after: equalIndex)...]
                    .trimmingCharacters(in: .whitespaces)
                currentProperties[key] = value
            }
        }

        // Save last section
        if let section = currentSection {
            sections.append((section, currentProperties))
        }

        return sections
    }
}
