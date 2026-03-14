import Foundation

/// Manages GCP configurations, accounts, and projects via gcloud CLI
actor GCPConfigService {
    private let shell: ShellService

    init(shell: ShellService) {
        self.shell = shell
    }

    // MARK: - Configurations

    /// Load all gcloud configurations
    func loadConfigurations() async -> [GCPConfiguration] {
        do {
            let result = try await shell.run("gcloud config configurations list --format=json 2>/dev/null")
            guard result.succeeded, !result.stdout.isEmpty else { return [] }

            let data = Data(result.stdout.utf8)
            let decoded = try JSONDecoder().decode([GCPConfigurationJSON].self, from: data)
            return decoded.map { $0.toConfiguration() }
        } catch {
            print("Failed to load GCP configurations: \(error)")
            return []
        }
    }

    /// Activate a gcloud configuration
    func activate(configName: String) async -> Bool {
        do {
            let result = try await shell.run("gcloud config configurations activate \(configName) 2>/dev/null")
            return result.succeeded
        } catch {
            print("Failed to activate GCP config \(configName): \(error)")
            return false
        }
    }

    /// Create a new gcloud configuration
    func createConfiguration(name: String, account: String?, project: String?, region: String?) async -> Bool {
        do {
            // Create the configuration
            let createResult = try await shell.run("gcloud config configurations create \(name) 2>/dev/null")
            guard createResult.succeeded else { return false }

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

            return true
        } catch {
            print("Failed to create GCP config \(name): \(error)")
            return false
        }
    }

    /// Delete a gcloud configuration
    func deleteConfiguration(name: String) async -> Bool {
        do {
            let result = try await shell.run("gcloud config configurations delete \(name) --quiet 2>/dev/null")
            return result.succeeded
        } catch {
            print("Failed to delete GCP config \(name): \(error)")
            return false
        }
    }

    /// Set the active project for a configuration
    func setProject(_ projectId: String, configuration: String? = nil) async -> Bool {
        do {
            var command = "gcloud config set project \(projectId)"
            if let configuration {
                command += " --configuration=\(configuration)"
            }
            command += " 2>/dev/null"
            let result = try await shell.run(command)
            return result.succeeded
        } catch {
            print("Failed to set GCP project \(projectId): \(error)")
            return false
        }
    }

    /// Set the active account for a configuration
    func setAccount(_ account: String, configuration: String? = nil) async -> Bool {
        do {
            var command = "gcloud config set account \(account)"
            if let configuration {
                command += " --configuration=\(configuration)"
            }
            command += " 2>/dev/null"
            let result = try await shell.run(command)
            return result.succeeded
        } catch {
            print("Failed to set GCP account \(account): \(error)")
            return false
        }
    }

    // MARK: - Accounts

    /// List all authenticated gcloud accounts
    func listAuthenticatedAccounts() async -> [GCPAccount] {
        do {
            let result = try await shell.run("gcloud auth list --format=json 2>/dev/null")
            guard result.succeeded, !result.stdout.isEmpty else { return [] }

            let data = Data(result.stdout.utf8)
            let decoded = try JSONDecoder().decode([GCPAccountJSON].self, from: data)
            return decoded.map { $0.toAccount() }
        } catch {
            print("Failed to list GCP accounts: \(error)")
            return []
        }
    }

    /// Add a new Google account via browser login
    func addAccount() async -> Bool {
        do {
            let result = try await shell.run("gcloud auth login --brief 2>&1")
            return result.succeeded
        } catch {
            print("Failed to start GCP auth: \(error)")
            return false
        }
    }

    /// Revoke a Google account
    func revokeAccount(_ account: String) async -> Bool {
        do {
            let result = try await shell.run("gcloud auth revoke \(account) --quiet 2>/dev/null")
            return result.succeeded
        } catch {
            print("Failed to revoke GCP account \(account): \(error)")
            return false
        }
    }

    // MARK: - Projects

    /// List projects accessible by a given account
    /// Returns (projects, errorMessage) — errorMessage is non-nil if auth failed
    func listProjects(account: String? = nil) async -> ([GCPProject], String?) {
        do {
            var command = "gcloud projects list --format=json"
            if let account {
                command += " --account=\(account)"
            }
            command += " 2>&1"

            let result = try await shell.run(command)

            // Check for auth errors
            if !result.succeeded || result.stdout.contains("Reauthentication failed") || result.stdout.contains("There was a problem refreshing") {
                return ([], "auth_needed")
            }

            guard !result.stdout.isEmpty else { return ([], nil) }

            // Try to parse JSON (stdout might have error text before the JSON)
            let data = Data(result.stdout.utf8)
            let decoded = try JSONDecoder().decode([GCPProjectJSON].self, from: data)
            return (decoded.map { $0.toProject() }, nil)
        } catch {
            print("Failed to list GCP projects: \(error)")
            return ([], nil)
        }
    }

    /// Login to a specific Google account (opens browser)
    func loginAccount(_ account: String) async -> Bool {
        do {
            let result = try await shell.run("gcloud auth login \(account) --brief 2>&1")
            return result.succeeded
        } catch {
            print("Failed to login GCP account \(account): \(error)")
            return false
        }
    }

    /// Run application-default login for a project
    func applicationDefaultLogin(project: String) async -> Bool {
        do {
            let result = try await shell.run("gcloud auth application-default login --project=\(project) 2>&1")
            return result.succeeded
        } catch {
            print("Failed to run application-default login for project \(project): \(error)")
            return false
        }
    }
}
