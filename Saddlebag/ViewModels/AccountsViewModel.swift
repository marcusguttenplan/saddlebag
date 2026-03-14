import Foundation
import SwiftUI
import AppKit

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

    // MARK: - Services

    private let shell = ShellService()
    private let awsConfigService = AWSConfigService()
    private let gcpConfigService: GCPConfigService
    private let ssoSessionService: SSOSessionService
    private let userConfigService = UserConfigService()

    private var refreshTimer: Timer?

    init() {
        self.gcpConfigService = GCPConfigService(shell: shell)
        self.ssoSessionService = SSOSessionService(shell: shell)
    }

    // MARK: - Computed

    /// Menubar label — empty when all display options are off (icon-only)
    var menuBarLabel: String {
        var parts: [String] = []

        if userConfig.showAWSAccountInMenuBar, let activeProfile = userConfig.activeAWSProfile {
            parts.append(userConfig.displayLabel(for: activeProfile))
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
            parts.append(activeGCP.displayName)
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
            lastError = "Failed to load AWS config: \(error.localizedDescription)"
        }

        // Load SSO token statuses
        tokenStatuses = await ssoSessionService.loadTokenStatuses(for: ssoSessions)

        // Load GCP data concurrently
        async let configs = gcpConfigService.loadConfigurations()
        async let accounts = gcpConfigService.listAuthenticatedAccounts()
        gcpConfigurations = await configs
        gcpAccounts = await accounts

        // Load projects for each account concurrently
        await withTaskGroup(of: (String, [GCPProject], String?).self) { group in
            for account in gcpAccounts {
                group.addTask {
                    let (projects, error) = await self.gcpConfigService.listProjects(account: account.account)
                    return (account.account, projects, error)
                }
            }
            var results: [String: [GCPProject]] = [:]
            var authNeeded: Set<String> = []
            for await (account, projects, error) in group {
                results[account] = projects
                if error == "auth_needed" {
                    authNeeded.insert(account)
                }
            }
            self.gcpProjectsByAccount = results
            self.gcpAccountAuthNeeded = authNeeded
        }

        isLoading = false
    }

    /// Login to an AWS SSO profile
    func loginAWS(profile: AWSProfile) async {
        isLoggingIn = true
        let success = await ssoSessionService.login(profileName: profile.name)
        if success {
            await userConfigService.setActiveProfile(profile.name)
            await refresh()
        }
        isLoggingIn = false
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
        await userConfigService.setActiveProfile(profile.name)
        userConfig = await userConfigService.getConfig()
    }

    /// Switch active GCP configuration
    func switchGCP(to config: GCPConfiguration) async {
        let success = await gcpConfigService.activate(configName: config.name)
        if success {
            await refresh()
        }
    }

    /// Set active GCP project and account on the active configuration
    func setGCPProject(_ projectId: String, account: String? = nil, configuration: String? = nil) async {
        // Set the account first so the project is accessed with the right credentials
        if let account {
            _ = await gcpConfigService.setAccount(account, configuration: configuration)
        }
        let success = await gcpConfigService.setProject(projectId, configuration: configuration)
        if success {
            await refresh()
        }
    }

    /// Load projects for a GCP account (stores per-account)
    func loadGCPProjects(account: String? = nil) async {
        let (projects, _) = await gcpConfigService.listProjects(account: account)
        gcpProjects = projects
    }

    /// Re-authenticate a specific GCP account and reload its projects
    func loginGCPAccount(_ account: String) async {
        isLoading = true
        let success = await gcpConfigService.loginAccount(account)
        if success {
            gcpAccountAuthNeeded.remove(account)
            await refresh()
        }
        isLoading = false
    }

    /// Run gcloud auth login for a GCP project's account
    func gcloudAuthLogin(account: String) async {
        isLoading = true
        _ = await gcpConfigService.loginAccount(account)
        await refresh()
        isLoading = false
    }

    /// Run gcloud auth application-default login for a project
    func gcloudApplicationDefaultLogin(project: String) async {
        isLoading = true
        _ = await gcpConfigService.applicationDefaultLogin(project: project)
        isLoading = false
    }

    /// Add a new Google account via browser auth
    func addGCPAccount() async {
        isLoading = true
        let success = await gcpConfigService.addAccount()
        if success {
            await refresh()
        }
        isLoading = false
    }

    /// Revoke a Google account
    func revokeGCPAccount(_ account: String) async {
        let success = await gcpConfigService.revokeAccount(account)
        if success {
            await refresh()
        }
    }

    /// Create a new gcloud configuration (org profile)
    func createGCPConfig(name: String, account: String?, project: String?, region: String?) async {
        let success = await gcpConfigService.createConfiguration(
            name: name, account: account, project: project, region: region
        )
        if success {
            await refresh()
        }
    }

    /// Delete a gcloud configuration
    func deleteGCPConfig(name: String) async {
        let success = await gcpConfigService.deleteConfiguration(name: name)
        if success {
            await refresh()
        }
    }

    /// Set a human-readable label for a profile
    func setLabel(for profileName: String, label: String?) async {
        await userConfigService.setLabel(for: profileName, label: label)
        userConfig = await userConfigService.getConfig()
    }

    /// Set environment tag for a profile
    func setTag(for profileName: String, tag: ProfileTag?) async {
        await userConfigService.setTag(for: profileName, tag: tag)
        userConfig = await userConfigService.getConfig()
    }

    /// Toggle favorite status for a profile
    func toggleFavorite(_ profileName: String) async {
        await userConfigService.toggleFavorite(profileName)
        userConfig = await userConfigService.getConfig()
    }

    /// Create a new AWS SSO profile in ~/.aws/config
    func createAWSProfile(name: String, ssoSession: String, accountId: String, roleName: String, region: String) async {
        let success = awsConfigService.appendProfile(
            name: name,
            ssoSession: ssoSession,
            accountId: accountId,
            roleName: roleName,
            region: region
        )
        if success {
            await refresh()
        }
    }

    /// Add a custom tag
    func addCustomTag(name: String) async {
        await userConfigService.addCustomTag(name: name)
        userConfig = await userConfigService.getConfig()
    }

    /// Remove a custom tag
    func removeCustomTag(name: String) async {
        await userConfigService.removeCustomTag(name: name)
        userConfig = await userConfigService.getConfig()
    }

    /// Update menubar display settings
    func updateMenuBarDisplay(aws: Bool, time: Bool, gcp: Bool) async {
        await userConfigService.setMenuBarDisplay(aws: aws, time: time, gcp: gcp)
        userConfig = await userConfigService.getConfig()
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
            }
        }
    }
}
