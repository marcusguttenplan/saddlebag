import Foundation

/// Manages desk definitions from ~/.saddlebag/desks/*.toml
/// Handles loading, saving, and orchestrating desk switches.
public actor DeskService {
    private let shell: ShellService
    private let gcpConfigService: GCPConfigService
    private let userConfigService: UserConfigService

    private static var desksDir: String {
        "\(NSHomeDirectory())/.saddlebag/desks"
    }

    public init(shell: ShellService, gcpConfigService: GCPConfigService, userConfigService: UserConfigService) {
        self.shell = shell
        self.gcpConfigService = gcpConfigService
        self.userConfigService = userConfigService
    }

    // MARK: - Load

    /// Load all desk definitions from ~/.saddlebag/desks/
    public func loadAll() throws -> [Desk] {
        let dir = Self.desksDir
        let fm = FileManager.default

        guard fm.fileExists(atPath: dir) else {
            return []
        }

        let files = try fm.contentsOfDirectory(atPath: dir)
        return files
            .filter { $0.hasSuffix(".toml") }
            .compactMap { filename -> Desk? in
                let path = (dir as NSString).appendingPathComponent(filename)
                return try? loadTOML(at: path, id: String(filename.dropLast(5)))
            }
            .sorted { $0.name < $1.name }
    }

    /// Load a single desk from a TOML file
    private func loadTOML(at path: String, id: String) throws -> Desk {
        let content = try String(contentsOfFile: path, encoding: .utf8)
        return parseTOML(content, id: id)
    }

    // MARK: - Save

    /// Write a desk definition to ~/.saddlebag/desks/<id>.toml
    public func save(_ desk: Desk) throws {
        let dir = Self.desksDir
        try FileManager.default.createDirectory(atPath: dir, withIntermediateDirectories: true)

        let path = (dir as NSString).appendingPathComponent("\(desk.id).toml")
        let toml = generateTOML(desk)
        try toml.write(toFile: path, atomically: true, encoding: .utf8)
    }

    /// Delete a desk definition
    public func delete(_ desk: Desk) throws {
        let path = (Self.desksDir as NSString).appendingPathComponent("\(desk.id).toml")
        try FileManager.default.removeItem(atPath: path)
    }

    // MARK: - Switch

    /// Orchestrate switching to a desk: AWS profile → GCP config → write state
    public func switchTo(_ desk: Desk) async throws -> SharedState {
        // Set AWS profile
        if let awsProfile = desk.awsProfile {
            try await userConfigService.setActiveProfile(awsProfile)
        }

        // Activate GCP config
        if let gcpConfig = desk.gcpConfig {
            try await gcpConfigService.activate(configName: gcpConfig)
        }

        // Load SSH key into agent
        if let sshKey = desk.sshKey {
            let expandedPath = (sshKey as NSString).expandingTildeInPath
            _ = try? await shell.run("ssh-add --apple-use-keychain \(expandedPath)")
        }

        // Build and write shared state
        let state = SharedState(
            activeDesk: desk.id,
            awsProfile: desk.awsProfile,
            gcpConfig: desk.gcpConfig,
            gitEmail: desk.gitEmail
        )
        try state.write()

        return state
    }

    // MARK: - TOML Parser (minimal, handles our desk format)

    private func parseTOML(_ content: String, id: String) -> Desk {
        var sections: [String: [String: String]] = [:]
        var currentSection = ""

        for line in content.components(separatedBy: .newlines) {
            let trimmed = line.trimmingCharacters(in: .whitespaces)

            // Skip empty lines and comments
            if trimmed.isEmpty || trimmed.hasPrefix("#") { continue }

            // Section header
            if trimmed.hasPrefix("[") && trimmed.hasSuffix("]") {
                currentSection = String(trimmed.dropFirst().dropLast())
                if sections[currentSection] == nil {
                    sections[currentSection] = [:]
                }
                continue
            }

            // Key = value
            if let eqIdx = trimmed.firstIndex(of: "=") {
                let key = trimmed[..<eqIdx].trimmingCharacters(in: .whitespaces)
                var value = trimmed[trimmed.index(after: eqIdx)...].trimmingCharacters(in: .whitespaces)
                // Strip quotes
                if value.hasPrefix("\"") && value.hasSuffix("\"") {
                    value = String(value.dropFirst().dropLast())
                }
                sections[currentSection, default: [:]][key] = value
            }
        }

        // Parse env vars
        var envVars: [String: String] = [:]
        if let envSection = sections["env"] {
            envVars = envSection
        }

        return Desk(
            id: id,
            name: sections["desk"]?["name"] ?? id,
            awsProfile: sections["aws"]?["profile"],
            gcpConfig: sections["gcp"]?["config"],
            gitEmail: sections["git"]?["email"],
            gitName: sections["git"]?["name"],
            sshKey: sections["ssh"]?["key"],
            workingDir: sections["desk"]?["working_dir"],
            envVars: envVars
        )
    }

    // MARK: - TOML Generator

    private func generateTOML(_ desk: Desk) -> String {
        var lines: [String] = []

        lines.append("[desk]")
        lines.append("name = \"\(desk.name)\"")
        if let dir = desk.workingDir {
            lines.append("working_dir = \"\(dir)\"")
        }

        if let profile = desk.awsProfile {
            lines.append("")
            lines.append("[aws]")
            lines.append("profile = \"\(profile)\"")
        }

        if let config = desk.gcpConfig {
            lines.append("")
            lines.append("[gcp]")
            lines.append("config = \"\(config)\"")
        }

        if desk.gitEmail != nil || desk.gitName != nil {
            lines.append("")
            lines.append("[git]")
            if let email = desk.gitEmail {
                lines.append("email = \"\(email)\"")
            }
            if let name = desk.gitName {
                lines.append("name = \"\(name)\"")
            }
        }

        if let key = desk.sshKey {
            lines.append("")
            lines.append("[ssh]")
            lines.append("key = \"\(key)\"")
        }

        if !desk.envVars.isEmpty {
            lines.append("")
            lines.append("[env]")
            for (key, value) in desk.envVars.sorted(by: { $0.key < $1.key }) {
                lines.append("\(key) = \"\(value)\"")
            }
        }

        lines.append("")
        return lines.joined(separator: "\n")
    }
}
