import Foundation
import SwiftUI
import AppKit
import SaddlebagShared

/// ViewModel managing secrets browsing, copying, and env file generation
@MainActor
@Observable
final class SecretsViewModel {
    // MARK: - State

    /// All secrets loaded from the selected project
    var secrets: [Secret] = []

    /// Currently selected GCP project ID
    var selectedProject: String?

    /// Multi-selected secret IDs (for bulk operations)
    var selectedSecrets: Set<String> = []

    /// In-memory cache of fetched secret values (cleared on project switch)
    var secretValues: [String: String] = [:]

    /// Search text filter
    var filterText: String = ""

    /// Label filters
    var filterOrg: String?
    var filterService: String?
    var filterStage: String?

    /// Loading / error state
    var isLoading = false
    var lastError: String?
    var clipboardToast: String?

    // MARK: - Computed

    /// Filtered secrets based on search text and label filters
    var filteredSecrets: [Secret] {
        secrets.filter { secret in
            // Text search
            if !filterText.isEmpty {
                let searchable = [
                    secret.name,
                    secret.org ?? "",
                    secret.service ?? "",
                    secret.stage ?? "",
                    secret.varName ?? ""
                ].joined(separator: " ").lowercased()

                if !searchable.contains(filterText.lowercased()) {
                    return false
                }
            }

            // Label filters
            if let org = filterOrg, !org.isEmpty, secret.org != org {
                return false
            }
            if let service = filterService, !service.isEmpty, secret.service != service {
                return false
            }
            if let stage = filterStage, !stage.isEmpty, secret.stage != stage {
                return false
            }

            return true
        }
        .sorted { $0.name < $1.name }
    }

    /// Unique org values from loaded secrets (for filter dropdown)
    var availableOrgs: [String] {
        Array(Set(secrets.compactMap(\.org))).sorted()
    }

    /// Unique service values from loaded secrets (for filter dropdown)
    var availableServices: [String] {
        Array(Set(secrets.compactMap(\.service))).sorted()
    }

    /// Unique stage values from loaded secrets (for filter dropdown)
    var availableStages: [String] {
        Array(Set(secrets.compactMap(\.stage))).sorted()
    }

    /// Secrets matching current selection
    var selectedSecretObjects: [Secret] {
        secrets.filter { selectedSecrets.contains($0.id) }
    }

    // MARK: - Services

    private let secretManagerService: SecretManagerService

    init(shell: ShellService) {
        self.secretManagerService = SecretManagerService(shell: shell)
    }

    // MARK: - Loading

    /// Load secrets for the selected project
    func loadSecrets() async {
        guard let project = selectedProject, !project.isEmpty else {
            secrets = []
            return
        }

        isLoading = true
        lastError = nil

        do {
            secrets = try await secretManagerService.listSecrets(project: project)
        } catch {
            lastError = "Failed to load secrets: \(error.localizedDescription)"
            secrets = []
        }

        isLoading = false
    }

    /// Switch to a different project — clears caches and reloads
    func switchProject(to projectId: String) async {
        selectedProject = projectId
        selectedSecrets = []
        secretValues = [:]
        filterOrg = nil
        filterService = nil
        filterStage = nil
        filterText = ""
        await loadSecrets()
    }

    // MARK: - Fetch Values

    /// Fetch the value for a single secret (caches in memory)
    func fetchValue(for secret: Secret) async -> String? {
        // Return cached value if available
        if let cached = secretValues[secret.name] {
            return cached
        }

        do {
            let value = try await secretManagerService.getSecretValue(
                name: secret.name,
                project: secret.project
            )
            secretValues[secret.name] = value
            return value
        } catch {
            lastError = "Failed to fetch \(secret.name): \(error.localizedDescription)"
            return nil
        }
    }

    /// Fetch values for all selected secrets
    func fetchSelectedValues() async {
        let toFetch = selectedSecretObjects.filter { secretValues[$0.name] == nil }
        guard !toFetch.isEmpty else { return }

        isLoading = true
        guard let project = selectedProject else {
            isLoading = false
            return
        }

        let values = await secretManagerService.getSecretValues(
            secrets: toFetch,
            project: project
        )
        for (name, value) in values {
            secretValues[name] = value
        }

        isLoading = false
    }

    // MARK: - Clipboard Operations

    /// Copy a single secret's raw value to clipboard (no key, just value)
    func copyValue(for secret: Secret) async {
        guard let value = await fetchValue(for: secret) else { return }
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(value, forType: .string)
        clipboardToast = "Value copied to clipboard"
    }

    /// Copy selected secrets as grouped env file content to clipboard
    func copySelectedAsEnv() async {
        await fetchSelectedValues()

        let secrets = selectedSecretObjects
        let formatted = SecretManagerService.formatAsEnvFiles(
            secrets: secrets,
            values: secretValues
        )

        if formatted.isEmpty {
            clipboardToast = "No secrets with service/stage/var labels selected"
            return
        }

        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(formatted, forType: .string)
        clipboardToast = "Copied \(secrets.count) secrets as .env"
    }

    /// Copy selected secrets as JSON to clipboard
    func copySelectedAsJSON() async {
        await fetchSelectedValues()

        let formatted = SecretManagerService.formatAsJSON(
            secrets: selectedSecretObjects,
            values: secretValues
        )

        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(formatted, forType: .string)
        clipboardToast = "Copied \(selectedSecretObjects.count) secrets as JSON"
    }

    // MARK: - Env File Generation

    /// Generate .env files on disk from selected secrets
    func generateEnvFiles(basePath: String) async {
        await fetchSelectedValues()

        isLoading = true
        do {
            let files = try secretManagerService.generateEnvFiles(
                secrets: selectedSecretObjects,
                values: secretValues,
                basePath: basePath
            )

            if files.isEmpty {
                lastError = "No secrets with service/stage/var labels to write"
            } else {
                clipboardToast = "Generated \(files.count) env file(s)"
            }
        } catch {
            lastError = "Failed to generate env files: \(error.localizedDescription)"
        }
        isLoading = false
    }

    // MARK: - Create Secret

    /// Create a new secret with labels
    func createSecret(
        name: String,
        value: String,
        org: String?,
        service: String?,
        stage: String?,
        varName: String?
    ) async {
        guard let project = selectedProject else {
            lastError = "No project selected"
            return
        }

        var labels: [String: String] = [:]
        if let org, !org.isEmpty { labels["org"] = org }
        if let service, !service.isEmpty { labels["service"] = service }
        if let stage, !stage.isEmpty { labels["stage"] = stage }
        if let varName, !varName.isEmpty { labels["var"] = varName }

        isLoading = true
        do {
            try await secretManagerService.createSecret(
                name: name,
                project: project,
                labels: labels,
                value: value
            )
            await loadSecrets()
            clipboardToast = "Created secret \(name)"
        } catch {
            lastError = "Failed to create secret: \(error.localizedDescription)"
        }
        isLoading = false
    }

    // MARK: - Selection Helpers

    func toggleSelection(_ secretId: String) {
        if selectedSecrets.contains(secretId) {
            selectedSecrets.remove(secretId)
        } else {
            selectedSecrets.insert(secretId)
        }
    }

    func selectAll() {
        selectedSecrets = Set(filteredSecrets.map(\.id))
    }

    func deselectAll() {
        selectedSecrets = []
    }

    var allFilteredSelected: Bool {
        !filteredSecrets.isEmpty && filteredSecrets.allSatisfy { selectedSecrets.contains($0.id) }
    }
}
