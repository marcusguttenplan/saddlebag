import Foundation

/// Manages GCP configurations, accounts, and projects via gcloud CLI
public actor GCPConfigService {
    private let shell: ShellService

    public init(shell: ShellService) {
        self.shell = shell
    }

    // MARK: - Configurations

    /// Load all gcloud configurations
    public func loadConfigurations() async throws -> [GCPConfiguration] {
        let result = try await shell.run("gcloud config configurations list --format=json 2>/dev/null")
        guard !result.stdout.isEmpty else { return [] }

        let data = Data(result.stdout.utf8)
        let decoded = try JSONDecoder().decode([GCPConfigurationJSON].self, from: data)
        return decoded.map { $0.toConfiguration() }
    }

    /// Activate a gcloud configuration
    public func activate(configName: String) async throws {
        _ = try await shell.run("gcloud config configurations activate \(configName) 2>/dev/null")
    }

    /// Create a new gcloud configuration
    public func createConfiguration(name: String, account: String?, project: String?, region: String?) async throws {
        // Create the configuration
        _ = try await shell.run("gcloud config configurations create \(name) 2>/dev/null")

        // Set account if provided
        if let account, !account.isEmpty {
            _ = try await shell.run("gcloud config set account \(account) --configuration=\(name) 2>/dev/null")
        }

        // Set project if provided
        if let project, !project.isEmpty {
            _ = try await shell.run("gcloud config set project \(project) --configuration=\(name) 2>/dev/null")
        }

        // Set region if provided
        if let region, !region.isEmpty {
            _ = try await shell.run("gcloud config set compute/region \(region) --configuration=\(name) 2>/dev/null")
        }
    }

    /// Delete a gcloud configuration
    public func deleteConfiguration(name: String) async throws {
        _ = try await shell.run("gcloud config configurations delete \(name) --quiet 2>/dev/null")
    }

    /// Set the active project for a configuration
    public func setProject(_ projectId: String, configuration: String? = nil) async throws {
        var command = "gcloud config set project \(projectId)"
        if let configuration {
            command += " --configuration=\(configuration)"
        }
        command += " 2>/dev/null"
        _ = try await shell.run(command)
    }

    /// Set the active account for a configuration
    public func setAccount(_ account: String, configuration: String? = nil) async throws {
        var command = "gcloud config set account \(account)"
        if let configuration {
            command += " --configuration=\(configuration)"
        }
        command += " 2>/dev/null"
        _ = try await shell.run(command)
    }

    // MARK: - Accounts

    /// List all authenticated gcloud accounts
    public func listAuthenticatedAccounts() async throws -> [GCPAccount] {
        let result = try await shell.run("gcloud auth list --format=json 2>/dev/null")
        guard !result.stdout.isEmpty else { return [] }

        let data = Data(result.stdout.utf8)
        let decoded = try JSONDecoder().decode([GCPAccountJSON].self, from: data)
        return decoded.map { $0.toAccount() }
    }

    /// Add a new Google account via browser login
    public func addAccount() async throws {
        _ = try await shell.run("gcloud auth login --brief 2>&1")
    }

    /// Revoke a Google account
    public func revokeAccount(_ account: String) async throws {
        _ = try await shell.run("gcloud auth revoke \(account) --quiet 2>/dev/null")
    }

    // MARK: - Projects

    /// List projects accessible by a given account
    /// Throws SaddlebagError.unauthenticated if reauthentication is needed.
    public func listProjects(account: String? = nil) async throws -> [GCPProject] {
        var command = "gcloud projects list --format=json"
        if let account {
            command += " --account=\(account)"
        }

        do {
            let result = try await shell.run(command)
            guard !result.stdout.isEmpty else { return [] }

            // Check if stdout contains an error message instead of JSON
            let trimmed = result.stdout.trimmingCharacters(in: .whitespacesAndNewlines)
            if !trimmed.hasPrefix("[") {
                if trimmed.contains("Reauthentication") || trimmed.contains("refreshing") {
                    throw SaddlebagError.unauthenticated(service: "Google Cloud")
                }
                return []
            }

            let data = Data(result.stdout.utf8)
            let decoded = try JSONDecoder().decode([GCPProjectJSON].self, from: data)
            return decoded.map { $0.toProject() }
        } catch SaddlebagError.shellExecutionFailed(_, _, let stderr) {
            if stderr.contains("Reauthentication") || stderr.contains("refreshing") {
                throw SaddlebagError.unauthenticated(service: "Google Cloud")
            }
            throw SaddlebagError.commandFailed(reason: stderr)
        } catch let error as SaddlebagError {
            // Re-throw SaddlebagError (including unauthenticated thrown above)
            throw error
        }
    }

    /// Login to a specific Google account (opens browser)
    public func loginAccount(_ account: String) async throws {
        _ = try await shell.run("gcloud auth login \(account) --brief 2>&1")
    }

    /// Run application-default login for a project
    public func applicationDefaultLogin(project: String) async throws {
        _ = try await shell.run("gcloud auth application-default login --project=\(project) 2>&1")
    }
}
