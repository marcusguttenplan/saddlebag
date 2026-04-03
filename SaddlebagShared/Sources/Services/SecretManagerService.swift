import Foundation

/// Manages secrets from GCP Secret Manager via gcloud CLI
public actor SecretManagerService {
    private let shell: ShellService

    public init(shell: ShellService) {
        self.shell = shell
    }

    // MARK: - List Secrets

    /// List all secrets in a GCP project
    public func listSecrets(project: String) async throws -> [Secret] {
        let command = "gcloud secrets list --project=\(project) --format=json --quiet"

        do {
            let result = try await shell.run(command)
            guard !result.stdout.isEmpty else { return [] }

            // Check if stdout contains an error instead of JSON
            let trimmed = result.stdout.trimmingCharacters(in: .whitespacesAndNewlines)
            if !trimmed.hasPrefix("[") {
                if trimmed.contains("Reauthentication") || trimmed.contains("refreshing") {
                    throw SaddlebagError.unauthenticated(service: "Google Cloud")
                }
                return []
            }

            let data = Data(result.stdout.utf8)
            let decoded = try JSONDecoder().decode([GCPSecretJSON].self, from: data)
            return decoded.map { $0.toSecret(project: project) }
        } catch let error as SaddlebagError {
            throw error
        } catch SaddlebagError.shellExecutionFailed(_, _, let stderr) {
            if stderr.contains("PERMISSION_DENIED") {
                throw SaddlebagError.commandFailed(reason: "Permission denied for project \(project). Check your IAM roles.")
            }
            if stderr.contains("Reauthentication") || stderr.contains("refreshing") {
                throw SaddlebagError.unauthenticated(service: "Google Cloud")
            }
            throw SaddlebagError.commandFailed(reason: stderr)
        }
    }

    // MARK: - Get Secret Value

    /// Retrieve the value of a specific secret version
    public func getSecretValue(
        name: String,
        project: String,
        version: String = "latest"
    ) async throws -> String {
        let command = "gcloud secrets versions access \(version) --secret=\(name) --project=\(project) 2>/dev/null"
        let result = try await shell.run(command)
        return result.stdout
    }

    /// Retrieve values for multiple secrets concurrently
    public func getSecretValues(
        secrets: [Secret],
        project: String
    ) async -> [String: String] {
        await withTaskGroup(of: (String, String?).self) { group in
            for secret in secrets {
                group.addTask {
                    let value = try? await self.getSecretValue(name: secret.name, project: project)
                    return (secret.name, value)
                }
            }

            var results: [String: String] = [:]
            for await (name, value) in group {
                if let value {
                    results[name] = value
                }
            }
            return results
        }
    }

    // MARK: - Create Secret

    /// Create a new secret with labels and an initial value
    public func createSecret(
        name: String,
        project: String,
        labels: [String: String],
        value: String
    ) async throws {
        // Build labels argument
        var command = "gcloud secrets create \(name) --project=\(project)"
        if !labels.isEmpty {
            let labelPairs = labels.map { "\($0.key)=\($0.value)" }.joined(separator: ",")
            command += " --labels=\(labelPairs)"
        }
        command += " 2>/dev/null"

        _ = try await shell.run(command)

        // Add the initial version with the value
        let addCommand = "printf '%s' '\(escapeForShell(value))' | gcloud secrets versions add \(name) --project=\(project) --data-file=- 2>/dev/null"
        _ = try await shell.run(addCommand)
    }

    // MARK: - Update Labels

    /// Update labels on an existing secret
    public func updateSecretLabels(
        name: String,
        project: String,
        labels: [String: String]
    ) async throws {
        guard !labels.isEmpty else { return }

        let labelPairs = labels.map { "\($0.key)=\($0.value)" }.joined(separator: ",")
        let command = "gcloud secrets update \(name) --project=\(project) --update-labels=\(labelPairs) 2>/dev/null"
        _ = try await shell.run(command)
    }

    // MARK: - Delete Secret

    /// Delete a secret
    public func deleteSecret(name: String, project: String) async throws {
        let command = "gcloud secrets delete \(name) --project=\(project) --quiet 2>/dev/null"
        _ = try await shell.run(command)
    }

    // MARK: - Env File Generation

    /// Generate .env files from secrets grouped by (service, stage) labels
    /// Writes files to: basePath/service/.env.stage
    public nonisolated func generateEnvFiles(
        secrets: [Secret],
        values: [String: String],
        basePath: String
    ) throws -> [String] {
        // Group secrets by (service, stage)
        var grouped: [String: [String: [(varName: String, value: String)]]] = [:]

        for secret in secrets {
            guard let service = secret.service,
                  let stage = secret.stage,
                  let value = values[secret.name] else {
                continue
            }
            grouped[service, default: [:]][stage, default: []].append((varName: secret.name, value: value))
        }

        var writtenFiles: [String] = []
        let fileManager = FileManager.default

        for (service, stages) in grouped.sorted(by: { $0.key < $1.key }) {
            for (stage, vars) in stages.sorted(by: { $0.key < $1.key }) {
                let dirPath = "\(basePath)/\(service)"
                let filePath = "\(dirPath)/.env.\(stage)"

                // Ensure directory exists
                try fileManager.createDirectory(
                    atPath: dirPath,
                    withIntermediateDirectories: true
                )

                // Build file content
                var lines: [String] = [
                    "# Generated by Saddlebag — do not edit",
                    "# service=\(service) stage=\(stage)",
                    ""
                ]
                for entry in vars.sorted(by: { $0.varName < $1.varName }) {
                    lines.append("\(entry.varName)=\"\(entry.value)\"")
                }
                lines.append("") // trailing newline

                let content = lines.joined(separator: "\n")
                try content.write(toFile: filePath, atomically: true, encoding: .utf8)
                writtenFiles.append(filePath)
            }
        }

        return writtenFiles
    }

    // MARK: - Clipboard Formatting

    /// Format secrets as grouped env file content for clipboard
    /// Groups by (service, stage) with section headers
    public static func formatAsEnvFiles(
        secrets: [Secret],
        values: [String: String]
    ) -> String {
        // Group secrets by (service, stage)
        var grouped: [String: [String: [(varName: String, value: String)]]] = [:]

        for secret in secrets {
            guard let service = secret.service,
                  let stage = secret.stage,
                  let value = values[secret.name] else {
                continue
            }
            grouped[service, default: [:]][stage, default: []].append((varName: secret.name, value: value))
        }

        var sections: [String] = []

        for (service, stages) in grouped.sorted(by: { $0.key < $1.key }) {
            for (stage, vars) in stages.sorted(by: { $0.key < $1.key }) {
                var lines: [String] = ["# --- \(service)/.env.\(stage) ---"]
                for entry in vars.sorted(by: { $0.varName < $1.varName }) {
                    lines.append("\(entry.varName)=\"\(entry.value)\"")
                }
                sections.append(lines.joined(separator: "\n"))
            }
        }

        return sections.joined(separator: "\n\n")
    }

    /// Format secrets as JSON for clipboard
    public static func formatAsJSON(
        secrets: [Secret],
        values: [String: String]
    ) -> String {
        var entries: [[String: Any]] = []
        for secret in secrets {
            var entry: [String: Any] = [
                "name": secret.name,
                "project": secret.project,
                "labels": secret.labels
            ]
            if let value = values[secret.name] {
                entry["value"] = value
            }
            entries.append(entry)
        }

        guard let data = try? JSONSerialization.data(
            withJSONObject: entries,
            options: [.prettyPrinted, .sortedKeys]
        ) else {
            return "[]"
        }
        return String(data: data, encoding: .utf8) ?? "[]"
    }

    // MARK: - Helpers

    private func escapeForShell(_ value: String) -> String {
        value.replacingOccurrences(of: "'", with: "'\\''")
    }
}
