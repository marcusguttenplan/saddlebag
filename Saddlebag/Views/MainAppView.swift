import SwiftUI
import ServiceManagement
import SaddlebagShared

/// Sidebar navigation items for the main app window
enum AppSection: String, CaseIterable, Identifiable {
    case desks = "Desks"
    case aws = "AWS"
    case gcp = "GCP"
    case settings = "Settings"

    var id: String { rawValue }

    var icon: String {
        switch self {
        case .desks: return "desktopcomputer"
        case .aws: return "server.rack"
        case .gcp: return "globe"
        case .settings: return "gear"
        }
    }
}

/// Main app window — three-column layout
struct MainAppView: View {
    @Bindable var viewModel: AccountsViewModel
    @State private var selectedSection: AppSection? = .desks

    var body: some View {
        NavigationSplitView {
            sidebar
        } detail: {
            detailView
        }
        .navigationSplitViewStyle(.balanced)
        .navigationTitle(selectedSection?.rawValue ?? "Saddlebag")
        .navigationSubtitle(statusSubtitle)
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                Button {
                    NSApp.keyWindow?.firstResponder?.tryToPerform(
                        #selector(NSSplitViewController.toggleSidebar(_:)), with: nil
                    )
                } label: {
                    Image(systemName: "sidebar.leading")
                }
                .help("Toggle Sidebar")
            }
        }
        .frame(minWidth: 720, minHeight: 480)
        .task {
            await viewModel.refresh()
        }
        .alert("Error", isPresented: Binding(
            get: { viewModel.lastError != nil },
            set: { _ in viewModel.lastError = nil }
        ), presenting: viewModel.lastError) { _ in
            Button("OK", role: .cancel) { }
        } message: { errorMsg in
            Text(errorMsg)
        }
    }

    private var statusSubtitle: String {
        var parts: [String] = []
        if let desk = viewModel.activeDesk {
            parts.append(desk.name)
        }
        if let aws = viewModel.userConfig.activeAWSProfile {
            parts.append(viewModel.redact(aws, as: .generic))
        }
        return parts.isEmpty ? "" : parts.joined(separator: " · ")
    }

    // MARK: - Sidebar

    private var sidebar: some View {
        List(selection: $selectedSection) {
            Section {
                ForEach([AppSection.desks, .aws, .gcp], id: \.self) { section in
                    Label(section.rawValue, systemImage: section.icon)
                        .tag(section)
                }
            }
        }
        .listStyle(.sidebar)
        .toolbar(removing: .sidebarToggle)
        .navigationSplitViewColumnWidth(min: 160, ideal: 180, max: 220)
        .safeAreaInset(edge: .bottom, spacing: 0) {
            VStack(spacing: 0) {
                Divider()
                Button {
                    selectedSection = .settings
                } label: {
                    Label("Settings", systemImage: "gear")
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(.horizontal, 16)
                        .padding(.vertical, 10)
                        .contentShape(Rectangle())
                        .foregroundStyle(selectedSection == .settings ? .white : .primary)
                }
                .buttonStyle(.plain)
                .background(selectedSection == .settings ? Color.accentColor : Color.clear)
                .clipShape(RoundedRectangle(cornerRadius: 6))
                .padding(.horizontal, 8)
                .padding(.vertical, 6)
            }
        }
    }

    // MARK: - Detail View

    @ViewBuilder
    private var detailView: some View {
        switch selectedSection {
        case .desks:
            DeskSettingsView(viewModel: viewModel)
        case .aws:
            AWSContentView(viewModel: viewModel)
        case .gcp:
            GCPContentView(viewModel: viewModel)
        case .settings:
            GeneralSettingsView(viewModel: viewModel)
        case .none:
            Text("Select a section")
                .foregroundStyle(.secondary)
        }
    }
}

// MARK: - AWS Content View

struct AWSContentView: View {
    @Bindable var viewModel: AccountsViewModel

    @State private var showNewProfileForm = false
    @State private var newProfileName = ""
    @State private var newProfileSSOSession = ""
    @State private var newProfileAccountId = ""
    @State private var newProfileRoleName = ""
    @State private var newProfileRegion = "us-east-1"

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                // SSO Portals
                GroupBox("SSO Portals") {
                    VStack(alignment: .leading, spacing: 8) {
                        ForEach(viewModel.ssoSessions) { session in
                            ssoPortalRow(session: session)
                        }
                    }
                    .padding(8)
                }

                // Profile groups with tag/label editing
                ForEach(viewModel.groupedAWSProfiles, id: \.portal) { group in
                    GroupBox(viewModel.redact(group.portalDisplayName, as: .generic)) {
                        VStack(alignment: .leading, spacing: 4) {
                            ForEach(group.profiles) { profile in
                                AWSProfileSettingsRow(
                                    profile: profile,
                                    userConfig: viewModel.userConfig,
                                    onSetLabel: { label in
                                        Task { await viewModel.setLabel(for: profile.name, label: label) }
                                    },
                                    onSetTag: { tag in
                                        Task { await viewModel.setTag(for: profile.name, tag: tag) }
                                    },
                                    onToggleFavorite: {
                                        Task { await viewModel.toggleFavorite(profile.name) }
                                    }
                                )
                            }
                        }
                        .padding(8)
                    }
                }

                // All profiles with actions
                GroupBox("Profiles") {
                    VStack(alignment: .leading, spacing: 8) {
                        ForEach(viewModel.awsProfiles) { profile in
                            awsProfileRow(profile: profile)
                        }

                        Divider()

                        if showNewProfileForm {
                            newAWSProfileForm
                        } else {
                            Button("Add Profile…") { showNewProfileForm = true }
                        }
                    }
                    .padding(8)
                }

                Spacer()
            }
            .padding()
        }
    }

    private func ssoPortalRow(session: SSOSession) -> some View {
        let hasValidToken = viewModel.tokenStatuses[session.name]?.isExpired == false
        return HStack {
            Image(systemName: hasValidToken ? "checkmark.circle.fill" : "exclamationmark.circle")
                .foregroundStyle(hasValidToken ? .green : .orange)
            VStack(alignment: .leading, spacing: 1) {
                Text(viewModel.redact(session.portalDisplayName, as: .generic))
                    .font(.system(.body, weight: .medium))
                Text(viewModel.redact(session.startUrl, as: .url))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer()
            if let token = viewModel.tokenStatuses[session.name], !token.isExpired {
                Text(token.timeRemainingFormatted)
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(token.expiryStatus == .expiringSoon ? .orange : .secondary)
            }
            if !hasValidToken {
                Button("Login") {
                    Task { await viewModel.loginPortal(session.portalDomain) }
                }
                .font(.caption)
            }
        }
    }

    private func awsProfileRow(profile: AWSProfile) -> some View {
        let isActive = viewModel.userConfig.activeAWSProfile == profile.name
        return HStack {
            Image(systemName: isActive ? "checkmark.circle.fill" : "circle")
                .foregroundStyle(isActive ? .green : .secondary)
            VStack(alignment: .leading) {
                Text(viewModel.redact(profile.name, as: .generic))
                    .font(.system(.body, weight: .medium))
                Text([profile.roleLabel, viewModel.redact(profile.accountId, as: .accountId), profile.region].joined(separator: " · "))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer()
            Button("Activate") {
                Task { await viewModel.setActiveAWS(profile: profile) }
            }
            .font(.caption)
            .disabled(isActive)
        }
    }

    private var newAWSProfileForm: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("New Profile")
                .font(.caption)
                .foregroundStyle(.secondary)
            HStack(spacing: 8) {
                TextField("Profile Name", text: $newProfileName)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 140)
                TextField("SSO Session", text: $newProfileSSOSession)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 140)
            }
            HStack(spacing: 8) {
                TextField("Account ID", text: $newProfileAccountId)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 140)
                TextField("Role Name", text: $newProfileRoleName)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 140)
            }
            HStack(spacing: 8) {
                TextField("Region", text: $newProfileRegion)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 140)
            }
            HStack {
                Button("Create") {
                    Task {
                        await viewModel.createAWSProfile(
                            name: newProfileName,
                            ssoSession: newProfileSSOSession,
                            accountId: newProfileAccountId,
                            roleName: newProfileRoleName,
                            region: newProfileRegion
                        )
                        newProfileName = ""
                        newProfileSSOSession = ""
                        newProfileAccountId = ""
                        newProfileRoleName = ""
                        newProfileRegion = "us-east-1"
                        showNewProfileForm = false
                    }
                }
                .disabled(newProfileName.isEmpty || newProfileSSOSession.isEmpty || newProfileAccountId.isEmpty || newProfileRoleName.isEmpty)
                Button("Cancel") { showNewProfileForm = false }
            }
        }
    }
}

// MARK: - GCP Content View

struct GCPContentView: View {
    @Bindable var viewModel: AccountsViewModel

    @State private var showNewConfigForm = false
    @State private var newConfigName = ""
    @State private var newConfigAccount = ""
    @State private var newConfigProject = ""
    @State private var newConfigRegion = ""

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                // Authenticated accounts
                GroupBox("Authenticated Accounts") {
                    VStack(alignment: .leading, spacing: 8) {
                        ForEach(viewModel.gcpAccounts) { account in
                            gcpAccountRow(account: account)
                        }

                        Button("Add Google Account…") {
                            Task { await viewModel.addGCPAccount() }
                        }
                    }
                    .padding(8)
                }

                // Projects per account with settings
                ForEach(viewModel.gcpAccounts) { account in
                    GroupBox(viewModel.redact(account.account, as: .email)) {
                        VStack(alignment: .leading, spacing: 4) {
                            gcpAccountProjectsContent(account: account)
                        }
                        .padding(8)
                    }
                }

                // Configurations
                GroupBox("Configurations") {
                    VStack(alignment: .leading, spacing: 8) {
                        ForEach(viewModel.gcpConfigurations) { config in
                            gcpConfigRow(config: config)
                        }

                        Divider()

                        if showNewConfigForm {
                            newGCPConfigForm
                        } else {
                            Button("New Configuration…") { showNewConfigForm = true }
                        }
                    }
                    .padding(8)
                }

                Spacer()
            }
            .padding()
        }
    }

    private func gcpAccountRow(account: GCPAccount) -> some View {
        let isAuthenticated = !viewModel.gcpAccountAuthNeeded.contains(account.account)
        return HStack {
            Image(systemName: isAuthenticated ? "checkmark.circle.fill" : "exclamationmark.circle")
                .foregroundStyle(isAuthenticated ? .green : .orange)
            VStack(alignment: .leading, spacing: 1) {
                Text(viewModel.redact(account.account, as: .email))
                    .font(.system(.body, weight: .medium))
                Text(account.isActive ? "Active account" : "Authenticated")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer()
            if !isAuthenticated {
                Button("Login") {
                    Task { await viewModel.loginGCPAccount(account.account) }
                }
                .font(.caption)
            }
        }
    }

    @ViewBuilder
    private func gcpAccountProjectsContent(account: GCPAccount) -> some View {
        if viewModel.gcpAccountAuthNeeded.contains(account.account) {
            HStack(spacing: 6) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .font(.system(size: 10))
                    .foregroundStyle(.orange)
                Text("Re-authentication needed")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Button("Login") {
                    Task { await viewModel.loginGCPAccount(account.account) }
                }
                .font(.caption)
            }
        } else if let projects = viewModel.gcpProjectsByAccount[account.account], !projects.isEmpty {
            ForEach(projects) { project in
                GCPProjectSettingsRow(
                    project: project,
                    userConfig: viewModel.userConfig,
                    onSetLabel: { label in
                        Task { await viewModel.setLabel(for: "gcp-project:\(project.projectId)", label: label) }
                    },
                    onSetTag: { tag in
                        Task { await viewModel.setTag(for: "gcp-project:\(project.projectId)", tag: tag) }
                    },
                    onToggleFavorite: {
                        Task { await viewModel.toggleFavorite("gcp-project:\(project.projectId)") }
                    }
                )
            }
        } else {
            Text("No projects")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }

    private func gcpConfigRow(config: GCPConfiguration) -> some View {
        HStack {
            Image(systemName: config.isActive ? "checkmark.circle.fill" : "circle")
                .foregroundStyle(config.isActive ? .green : .secondary)
            VStack(alignment: .leading) {
                Text(viewModel.redact(config.name, as: .generic))
                    .font(.system(.body, weight: .medium))
                Text([config.project.map { viewModel.redact($0, as: .projectId) }, config.account.map { viewModel.redact($0, as: .email) }].compactMap { $0 }.joined(separator: " · "))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer()
            Button("Activate") {
                Task { await viewModel.switchGCP(to: config) }
            }
            .font(.caption)
            .disabled(config.isActive)
            Button {
                Task { await viewModel.deleteGCPConfig(name: config.name) }
            } label: {
                Image(systemName: "trash")
                    .foregroundStyle(.red.opacity(0.6))
            }
            .buttonStyle(.borderless)
            .disabled(config.isActive)
        }
    }

    private var newGCPConfigForm: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("New Configuration")
                .font(.caption)
                .foregroundStyle(.secondary)
            HStack(spacing: 8) {
                TextField("Name", text: $newConfigName)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 120)
                TextField("Account (email)", text: $newConfigAccount)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 180)
            }
            HStack(spacing: 8) {
                TextField("Project ID", text: $newConfigProject)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 150)
                TextField("Region", text: $newConfigRegion)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 150)
            }
            HStack {
                Button("Create") {
                    Task {
                        await viewModel.createGCPConfig(
                            name: newConfigName,
                            account: newConfigAccount.isEmpty ? nil : newConfigAccount,
                            project: newConfigProject.isEmpty ? nil : newConfigProject,
                            region: newConfigRegion.isEmpty ? nil : newConfigRegion
                        )
                        newConfigName = ""
                        newConfigAccount = ""
                        newConfigProject = ""
                        newConfigRegion = ""
                        showNewConfigForm = false
                    }
                }
                .disabled(newConfigName.isEmpty)
                Button("Cancel") { showNewConfigForm = false }
            }
        }
    }
}

// MARK: - General Settings View

struct GeneralSettingsView: View {
    @Bindable var viewModel: AccountsViewModel
    @State private var launchAtLogin = SMAppService.mainApp.status == .enabled
    @State private var shellInstallStatus: ShellInstallStatus = .unknown
    @State private var newCustomTagName = ""

    var body: some View {
        Form {
            Section("Menubar Display") {
                menubarDisplaySection
            }

            Section("Startup") {
                launchAtLoginSection
            }

            Section("Refresh") {
                refreshSection
            }

            Section("Shell Integration") {
                shellIntegrationSection
            }

            Section("Custom Tags") {
                customTagsSection
            }

            Section("Screenshot Mode") {
                screenshotModeSection
            }

            Section("About") {
                LabeledContent("Version", value: "2.0.0")
                LabeledContent("Config Path", value: "~/.saddlebag/")
            }
        }
        .formStyle(.grouped)
        .task {
            checkZshrcInstallation()
        }
    }

    private var menubarDisplaySection: some View {
        Group {
            Text("Choose what to show next to the icon:")
                .font(.caption)
                .foregroundStyle(.secondary)

            Toggle("Active AWS Account", isOn: Binding(
                get: { viewModel.userConfig.showAWSAccountInMenuBar },
                set: { newVal in
                    Task {
                        await viewModel.updateMenuBarDisplay(
                            aws: newVal,
                            time: viewModel.userConfig.showTimeRemainingInMenuBar,
                            gcp: viewModel.userConfig.showGCPProjectInMenuBar,
                            desk: viewModel.userConfig.showDeskInMenuBar
                        )
                    }
                }
            ))

            Toggle("Active GCP Project", isOn: Binding(
                get: { viewModel.userConfig.showGCPProjectInMenuBar },
                set: { newVal in
                    Task {
                        await viewModel.updateMenuBarDisplay(
                            aws: viewModel.userConfig.showAWSAccountInMenuBar,
                            time: viewModel.userConfig.showTimeRemainingInMenuBar,
                            gcp: newVal,
                            desk: viewModel.userConfig.showDeskInMenuBar
                        )
                    }
                }
            ))

            Toggle("Active Desk", isOn: Binding(
                get: { viewModel.userConfig.showDeskInMenuBar },
                set: { newVal in
                    Task {
                        await viewModel.updateMenuBarDisplay(
                            aws: viewModel.userConfig.showAWSAccountInMenuBar,
                            time: viewModel.userConfig.showTimeRemainingInMenuBar,
                            gcp: viewModel.userConfig.showGCPProjectInMenuBar,
                            desk: newVal
                        )
                    }
                }
            ))

            Toggle("Time Remaining", isOn: Binding(
                get: { viewModel.userConfig.showTimeRemainingInMenuBar },
                set: { newVal in
                    Task {
                        await viewModel.updateMenuBarDisplay(
                            aws: viewModel.userConfig.showAWSAccountInMenuBar,
                            time: newVal,
                            gcp: viewModel.userConfig.showGCPProjectInMenuBar,
                            desk: viewModel.userConfig.showDeskInMenuBar
                        )
                    }
                }
            ))

        }
    }

    private var launchAtLoginSection: some View {
        Group {
            Toggle("Open Saddlebag at login", isOn: Binding(
                get: { launchAtLogin },
                set: { newValue in
                    do {
                        if newValue {
                            try SMAppService.mainApp.register()
                        } else {
                            try SMAppService.mainApp.unregister()
                        }
                        launchAtLogin = newValue
                    } catch {
                        launchAtLogin = SMAppService.mainApp.status == .enabled
                    }
                }
            ))

            Text("Saddlebag will start automatically when you log in.")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }

    private var refreshSection: some View {
        Group {
            Picker("Check sessions every:", selection: Binding(
                get: { viewModel.userConfig.refreshInterval },
                set: { _ in }
            )) {
                Text("30 seconds").tag(30)
                Text("1 minute").tag(60)
                Text("5 minutes").tag(300)
            }

            Text("Token expiry is checked periodically to keep the menubar icon up to date.")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }

    private static let zshrcSnippet = """
    # Saddlebag: load active context
    if command -v sb &> /dev/null; then
        eval "$(sb init zsh)"
    fi
    """

    private static let zshrcMarker = "# Saddlebag: load active context"

    private var shellIntegrationSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Add this to your ~/.zshrc to sync environment variables automatically:")
                .font(.caption)
                .foregroundStyle(.secondary)

            Text(Self.zshrcSnippet)
                .font(.system(.caption, design: .monospaced))
                .padding(8)
                .background(.quaternary)
                .clipShape(RoundedRectangle(cornerRadius: 6))
                .textSelection(.enabled)

            HStack(spacing: 12) {
                Button {
                    installToZshrc()
                } label: {
                    switch shellInstallStatus {
                    case .unknown, .notInstalled:
                        Text("Install to ~/.zshrc")
                    case .alreadyInstalled:
                        Label("Already in ~/.zshrc", systemImage: "checkmark.circle.fill")
                    case .installed:
                        Label("Installed", systemImage: "checkmark.circle.fill")
                    case .error:
                        Label("Error", systemImage: "xmark.circle.fill")
                    }
                }
                .disabled(shellInstallStatus == .alreadyInstalled || shellInstallStatus == .installed)
            }
        }
    }

    private var customTagsSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Create custom tags to organize profiles and projects beyond the built-in tags.")
                .font(.caption)
                .foregroundStyle(.secondary)

            ForEach(viewModel.userConfig.customTags) { tag in
                HStack {
                    Text("🟣")
                    Text(tag.name)
                        .font(.system(.body, weight: .medium))
                    Spacer()
                    Button {
                        Task { await viewModel.removeCustomTag(name: tag.name) }
                    } label: {
                        Image(systemName: "xmark.circle")
                            .foregroundStyle(.red.opacity(0.6))
                    }
                    .buttonStyle(.borderless)
                }
            }

            HStack(spacing: 8) {
                TextField("Tag name", text: $newCustomTagName)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 160)
                    .onSubmit {
                        guard !newCustomTagName.isEmpty else { return }
                        Task {
                            await viewModel.addCustomTag(name: newCustomTagName)
                            newCustomTagName = ""
                        }
                    }
                Button("Create") {
                    Task {
                        await viewModel.addCustomTag(name: newCustomTagName)
                        newCustomTagName = ""
                    }
                }
                .disabled(newCustomTagName.isEmpty)
            }
        }
    }

    private var screenshotModeSection: some View {
        Group {
            Toggle("Obfuscate sensitive data", isOn: Binding(
                get: { viewModel.userConfig.screenshotMode },
                set: { _ in
                    Task { await viewModel.toggleScreenshotMode() }
                }
            ))

            Text("Hides account IDs, email addresses, SSO URLs, and project IDs so you can safely take screenshots.")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }

    // MARK: - Shell Integration Helpers

    private func checkZshrcInstallation() {
        let zshrcPath = NSHomeDirectory() + "/.zshrc"
        guard let content = try? String(contentsOfFile: zshrcPath, encoding: .utf8) else {
            shellInstallStatus = .notInstalled
            return
        }
        shellInstallStatus = content.contains(Self.zshrcMarker) ? .alreadyInstalled : .notInstalled
    }

    private func installToZshrc() {
        let zshrcPath = NSHomeDirectory() + "/.zshrc"

        if let content = try? String(contentsOfFile: zshrcPath, encoding: .utf8),
           content.contains(Self.zshrcMarker) {
            shellInstallStatus = .alreadyInstalled
            return
        }

        do {
            let handle = try FileHandle(forWritingTo: URL(fileURLWithPath: zshrcPath))
            handle.seekToEndOfFile()
            let snippet = "\n\n" + Self.zshrcSnippet + "\n"
            handle.write(snippet.data(using: .utf8)!)
            handle.closeFile()
            shellInstallStatus = .installed
        } catch {
            do {
                let snippet = Self.zshrcSnippet + "\n"
                try snippet.write(toFile: zshrcPath, atomically: true, encoding: .utf8)
                shellInstallStatus = .installed
            } catch {
                shellInstallStatus = .error
            }
        }
    }
}
