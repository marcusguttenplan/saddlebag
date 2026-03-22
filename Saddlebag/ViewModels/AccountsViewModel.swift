import Foundation
import SwiftUI
import AppKit
import SaddlebagShared

/// Central state manager for all cloud accounts
@MainActor
@Observable
final class AccountsViewModel {
    // MARK: - Published State

    var awsProfiles: [AWSProfile] = []
    var ssoSessions: [SSOSession] = []
    var tokenStatuses: [String: SSOTokenCache] = [:]
    var groupedAWSProfiles: [(portal: String, portalDisplayName: String, profiles: [AWSProfile])] = []

    var gcpConfigurations: [GCPConfiguration] = []
    var gcpAccounts: [GCPAccount] = []
    var gcpProjects: [GCPProject] = []
    var gcpProjectsByAccount: [String: [GCPProject]] = [:]
    var gcpAccountAuthNeeded: Set<String> = []

    var userConfig: UserConfig = .default
    var isLoading = false
    var isLoggingIn = false
    var lastError: String?

    // Desk state
    var desks: [Desk] = []
    var activeDesk: Desk?

    // MARK: - Services

    private let shell = ShellService()
    private let awsConfigService = AWSConfigService()
    private let gcpConfigService: GCPConfigService
    private let ssoSessionService: SSOSessionService
    private let userConfigService = UserConfigService()
    private let deskService: DeskService
    private let healthMonitor = CredentialHealthMonitor()

    private var refreshTimer: Timer?

    init() {
        self.gcpConfigService = GCPConfigService(shell: shell)
        self.ssoSessionService = SSOSessionService(shell: shell)
        self.deskService = DeskService(shell: shell, gcpConfigService: gcpConfigService, userConfigService: userConfigService)
        healthMonitor.requestPermission()
    }

    // MARK: - Computed

    /// Menubar label — empty when all display options are off (icon-only)
    var menuBarLabel: String {
        var parts: [String] = []

        // Show active desk name as first element
        if let desk = activeDesk {
            parts.append(desk.name)
        }

        if userConfig.showAWSAccountInMenuBar, let activeProfile = userConfig.activeAWSProfile {
            parts.append(redact(userConfig.displayLabel(for: activeProfile), as: .generic))
        }

        if userConfig.showTimeRemainingInMenuBar {
            let shortestExpiry = tokenStatuses.values
                .filter { !$0.isExpired }
                .min(by: { $0.timeRemaining < $1.timeRemaining })
            if let expiry = shortestExpiry {
                parts.append(expiry.timeRemainingFormatted)
            }
        }

        if userConfig.showGCPProjectInMenuBar,
           let activeGCP = gcpConfigurations.first(where: { $0.isActive }) {
            parts.append(redact(activeGCP.displayName, as: .projectId))
        }

        return parts.joined(separator: " · ")
    }

    /// Favorite profiles (from both AWS and GCP)
    var favoriteProfiles: [AWSProfile] {
        awsProfiles.filter { userConfig.isFavorite($0.name) }
    }

    /// Menubar icon based on session status
    var menuBarIcon: String {
        let hasExpiring = tokenStatuses.values.contains { $0.expiryStatus == .expiringSoon }
        let allExpired = !tokenStatuses.isEmpty && tokenStatuses.values.allSatisfy { $0.isExpired }

        if allExpired { return "exclamationmark.icloud" }
        if hasExpiring { return "icloud.slash" }
        return "cloud.fill"
    }

    // MARK: - Lifecycle

    func start() async {
        await refresh()
        startRefreshTimer()
    }

    func stop() {
        refreshTimer?.invalidate()
        refreshTimer = nil
    }

    // MARK: - Actions

    /// Refresh all data from disk and CLI
    func refresh() async {
        isLoading = true
        lastError = nil

        // Load user config
        userConfig = await userConfigService.getConfig()

        // Load AWS profiles
        do {
            let (profiles, sessions) = try awsConfigService.loadProfiles()
            self.awsProfiles = profiles
            self.ssoSessions = sessions
            self.groupedAWSProfiles = awsConfigService.groupedProfiles(
                profiles: profiles,
                sessions: sessions
            )
        } catch {
            // Fail open: don't clear profiles if we fail to read, but log error
            lastError = "Failed to load AWS config: \(error.localizedDescription)"
        }

        // Load SSO token statuses
        tokenStatuses = await ssoSessionService.loadTokenStatuses(for: ssoSessions)
        healthMonitor.evaluate(sessions: tokenStatuses)

        // Load GCP data concurrently
        do {
            let configs = try await gcpConfigService.loadConfigurations()
            self.gcpConfigurations = configs
        } catch {
            // fail open
        }

        do {
            let accounts = try await gcpConfigService.listAuthenticatedAccounts()
            self.gcpAccounts = accounts
        } catch {
            // fail open
        }

        // Load projects for each account concurrently
        await withTaskGroup(of: (String, [GCPProject], Error?).self) { group in
            for account in gcpAccounts {
                group.addTask {
                    do {
                        let projects = try await self.gcpConfigService.listProjects(account: account.account)
                        return (account.account, projects, nil)
                    } catch {
                        return (account.account, [], error)
                    }
                }
            }
            // Start with existing to fail open
            var results: [String: [GCPProject]] = self.gcpProjectsByAccount
            var authNeeded: Set<String> = self.gcpAccountAuthNeeded
            
            for await (account, projects, error) in group {
                if let error {
                    if case SaddlebagError.unauthenticated = error {
                        authNeeded.insert(account)
                    }
                } else {
                    results[account] = projects
                    authNeeded.remove(account)
                }
            }
            self.gcpProjectsByAccount = results
            self.gcpAccountAuthNeeded = authNeeded
        }

        isLoading = false
        await loadDesks()
        writeSharedState()
    }

    /// Login to an AWS SSO profile
    func loginAWS(profile: AWSProfile) async {
        isLoggingIn = true
        lastError = nil
        defer { isLoggingIn = false }

        // Copy SSO URL to clipboard as fallback if browser fails
        if let session = ssoSessions.first(where: { $0.name == profile.ssoSessionName }) {
            NSPasteboard.general.clearContents()
            NSPasteboard.general.setString(session.startUrl, forType: .string)
        }

        do {
            try await ssoSessionService.login(profileName: profile.name)
            try await userConfigService.setActiveProfile(profile.name)
            await refresh()
        } catch {
            lastError = "SSO login failed: \(error.localizedDescription). SSO URL copied to clipboard — paste in your preferred browser."
        }
    }

    /// Login to an entire SSO portal (authenticates all profiles under it)
    func loginPortal(_ portalDomain: String) async {
        // Find the first profile under this portal to use for login
        guard let group = groupedAWSProfiles.first(where: { $0.portal == portalDomain }),
              let firstProfile = group.profiles.first else {
            return
        }
        await loginAWS(profile: firstProfile)
    }

    /// Set a profile as the active AWS profile (without SSO login)
    func setActiveAWS(profile: AWSProfile) async {
        do {
            try await userConfigService.setActiveProfile(profile.name)
            userConfig = await userConfigService.getConfig()
            writeSharedState()
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Switch to a desk — orchestrates AWS, GCP, SSH, and state
    func switchDesk(_ desk: Desk) async {
        do {
            let state = try await deskService.switchTo(desk)
            activeDesk = desk
            // Refresh all data to reflect the new context
            await refresh()
            // Update userConfig with the new AWS profile
            userConfig = await userConfigService.getConfig()
        } catch {
            lastError = "Failed to switch desk: \(error.localizedDescription)"
        }
    }

    /// Switch to a desk by ID (used by IPC)
    func switchDeskByID(_ id: String) async -> Bool {
        guard let desk = desks.first(where: { $0.id == id }) else {
            lastError = "Desk '\(id)' not found"
            return false
        }
        await switchDesk(desk)
        return true
    }

    /// Load desk definitions from ~/.saddlebag/desks/
    func loadDesks() async {
        do {
            desks = try await deskService.loadAll()
            // Restore active desk from shared state
            let state = SharedState.read()
            if let activeDeskID = state.activeDesk {
                activeDesk = desks.first(where: { $0.id == activeDeskID })
            }
        } catch {
            // Non-fatal: desks are optional
            print("[Desks] Failed to load: \(error)")
        }
    }

    /// Save (create or update) a desk definition
    func saveDesk(_ desk: Desk) async {
        do {
            try await deskService.save(desk)
            await loadDesks()
        } catch {
            lastError = "Failed to save desk: \(error.localizedDescription)"
        }
    }

    /// Delete a desk definition
    func deleteDesk(_ desk: Desk) async {
        do {
            try await deskService.delete(desk)
            if activeDesk?.id == desk.id {
                activeDesk = nil
            }
            await loadDesks()
        } catch {
            lastError = "Failed to delete desk: \(error.localizedDescription)"
        }
    }

    /// Switch active GCP configuration
    func switchGCP(to config: GCPConfiguration) async {
        do {
            try await gcpConfigService.activate(configName: config.name)
            await refresh()
            writeSharedState()
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Set active GCP project and account on the active configuration
    func setGCPProject(_ projectId: String, account: String? = nil, configuration: String? = nil) async {
        do {
            // Set the account first so the project is accessed with the right credentials
            if let account {
                try await gcpConfigService.setAccount(account, configuration: configuration)
            }
            try await gcpConfigService.setProject(projectId, configuration: configuration)
            await refresh()
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Load projects for a GCP account (stores per-account)
    func loadGCPProjects(account: String? = nil) async {
        do {
            let projects = try await gcpConfigService.listProjects(account: account)
            gcpProjects = projects
        } catch {
            // fail open
        }
    }

    /// Re-authenticate a specific GCP account and reload its projects
    func loginGCPAccount(_ account: String) async {
        isLoading = true
        do {
            try await gcpConfigService.loginAccount(account)
            gcpAccountAuthNeeded.remove(account)
            await refresh()
        } catch {
            lastError = "Failed to login to \(account): \(error.localizedDescription)"
        }
        isLoading = false
    }

    /// Run gcloud auth login for a GCP project's account
    func gcloudAuthLogin(account: String) async {
        isLoading = true
        do {
            try await gcpConfigService.loginAccount(account)
            await refresh()
        } catch {
            lastError = "Failed to authenticate account (\(account)): \(error.localizedDescription)"
        }
        isLoading = false
    }

    /// Run gcloud auth application-default login for a project
    func gcloudApplicationDefaultLogin(project: String) async {
        isLoading = true
        do {
            try await gcpConfigService.applicationDefaultLogin(project: project)
        } catch {
            lastError = "Failed to run application-default login: \(error.localizedDescription)"
        }
        isLoading = false
    }

    /// Add a new Google account via browser auth
    func addGCPAccount() async {
        isLoading = true
        do {
            try await gcpConfigService.addAccount()
            await refresh()
        } catch {
            lastError = "Failed to add Google Account: \(error.localizedDescription)"
        }
        isLoading = false
    }

    /// Revoke a Google account
    func revokeGCPAccount(_ account: String) async {
        do {
            try await gcpConfigService.revokeAccount(account)
            await refresh()
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Create a new gcloud configuration (org profile)
    func createGCPConfig(name: String, account: String?, project: String?, region: String?) async {
        do {
            try await gcpConfigService.createConfiguration(
                name: name, account: account, project: project, region: region
            )
            await refresh()
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Delete a gcloud configuration
    func deleteGCPConfig(name: String) async {
        do {
            try await gcpConfigService.deleteConfiguration(name: name)
            await refresh()
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Set a human-readable label for a profile
    func setLabel(for profileName: String, label: String?) async {
        do {
            try await userConfigService.setLabel(for: profileName, label: label)
            userConfig = await userConfigService.getConfig()
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Set environment tag for a profile
    func setTag(for profileName: String, tag: ProfileTag?) async {
        do {
            try await userConfigService.setTag(for: profileName, tag: tag)
            userConfig = await userConfigService.getConfig()
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Toggle favorite status for a profile
    func toggleFavorite(_ profileName: String) async {
        do {
            try await userConfigService.toggleFavorite(profileName)
            userConfig = await userConfigService.getConfig()
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Create a new AWS SSO profile in ~/.aws/config
    func createAWSProfile(name: String, ssoSession: String, accountId: String, roleName: String, region: String) async {
        do {
            try awsConfigService.appendProfile(
                name: name,
                ssoSession: ssoSession,
                accountId: accountId,
                roleName: roleName,
                region: region
            )
            await refresh()
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Add a custom tag
    func addCustomTag(name: String) async {
        do {
            try await userConfigService.addCustomTag(name: name)
            userConfig = await userConfigService.getConfig()
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Remove a custom tag
    func removeCustomTag(name: String) async {
        do {
            try await userConfigService.removeCustomTag(name: name)
            userConfig = await userConfigService.getConfig()
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Update menubar display settings
    func updateMenuBarDisplay(aws: Bool, time: Bool, gcp: Bool) async {
        do {
            try await userConfigService.setMenuBarDisplay(aws: aws, time: time, gcp: gcp)
            userConfig = await userConfigService.getConfig()
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Toggle screenshot mode for data obfuscation
    func toggleScreenshotMode() async {
        do {
            try await userConfigService.setScreenshotMode(!userConfig.screenshotMode)
            userConfig = await userConfigService.getConfig()
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Redact a value if screenshot mode is active
    func redact(_ value: String, as kind: SensitiveDataKind) -> String {
        guard userConfig.screenshotMode else { return value }
        return Obfuscator.redact(value, as: kind)
    }


    func copyEnvVars(for profile: AWSProfile) {
        let envVars = [
            "export AWS_PROFILE=\(profile.name)",
            "export AWS_REGION=\(profile.region)",
            "export AWS_DEFAULT_REGION=\(profile.region)"
        ].joined(separator: "\n")

        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(envVars, forType: .string)
    }

    /// Copy GCP environment variables to clipboard
    func copyEnvVars(for config: GCPConfiguration) {
        var envVars: [String] = []
        if let project = config.project {
            envVars.append("export GOOGLE_CLOUD_PROJECT=\(project)")
            envVars.append("export GCLOUD_PROJECT=\(project)")
        }
        if let account = config.account {
            envVars.append("export GOOGLE_CLOUD_ACCOUNT=\(account)")
        }
        if let region = config.region {
            envVars.append("export GOOGLE_CLOUD_REGION=\(region)")
        }

        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(envVars.joined(separator: "\n"), forType: .string)
    }

    /// Open AWS Console for a profile
    func openAWSConsole(for profile: AWSProfile) {
        let url = "https://\(profile.region).console.aws.amazon.com/console/home?region=\(profile.region)"
        if let nsUrl = URL(string: url) {
            NSWorkspace.shared.open(nsUrl)
        }
    }

    /// Open GCP Console for a config
    func openGCPConsole(for config: GCPConfiguration) {
        var url = "https://console.cloud.google.com"
        if let project = config.project {
            url += "/home/dashboard?project=\(project)"
        }
        if let nsUrl = URL(string: url) {
            NSWorkspace.shared.open(nsUrl)
        }
    }

    // MARK: - Shared State

    /// Write current state to ~/.saddlebag/state.json for CLI
    private func writeSharedState() {
        let activeGCPConfig = gcpConfigurations.first(where: { $0.isActive })?.name
        let state = SharedState(
            activeDesk: activeDesk?.id,
            awsProfile: userConfig.activeAWSProfile,
            gcpConfig: activeGCPConfig
        )
        do {
            try state.write()
        } catch {
            print("[SharedState] Failed to write: \(error)")
        }
    }

    // MARK: - Private

    private func startRefreshTimer() {
        refreshTimer?.invalidate()
        refreshTimer = Timer.scheduledTimer(
            withTimeInterval: TimeInterval(userConfig.refreshInterval),
            repeats: true
        ) { [weak self] _ in
            guard let self else { return }
            Task { @MainActor in
                // Only refresh token statuses, not full config reload
                self.tokenStatuses = await self.ssoSessionService.loadTokenStatuses(for: self.ssoSessions)
                self.healthMonitor.evaluate(sessions: self.tokenStatuses)
            }
        }
    }
}
