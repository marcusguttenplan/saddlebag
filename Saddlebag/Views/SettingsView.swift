import SwiftUI
import ServiceManagement
import SaddlebagShared

/// Settings window with tabs for AWS, GCP, and General preferences
struct SettingsView: View {
    @Bindable var viewModel: AccountsViewModel
    @State private var launchAtLogin = SMAppService.mainApp.status == .enabled

    var body: some View {
        TabView {
            Tab("AWS", systemImage: "server.rack") {
                awsTab
            }

            Tab("GCP", systemImage: "globe") {
                gcpTab
            }

            Tab("General", systemImage: "gear") {
                generalTab
            }
        }
        .frame(width: 580, height: 460)
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

    // MARK: - AWS Tab

    @State private var showNewProfileForm = false
    @State private var newProfileName = ""
    @State private var newProfileSSOSession = ""
    @State private var newProfileAccountId = ""
    @State private var newProfileRoleName = ""
    @State private var newProfileRegion = "us-east-1"

    private var awsTab: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                awsSSOPortalsSection
                awsProfileGroupsSection
                awsProfilesListSection
                Spacer()
            }
            .padding(.top)
        }
    }

    private var awsSSOPortalsSection: some View {
        GroupBox("SSO Portals") {
            VStack(alignment: .leading, spacing: 8) {
                ForEach(viewModel.ssoSessions) { session in
                    awsSSOPortalRow(session: session)
                }
            }
            .padding(8)
        }
        .padding(.horizontal)
    }

    private func awsSSOPortalRow(session: SSOSession) -> some View {
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
            if !hasValidToken {
                Button("Login") {
                    Task { await viewModel.loginPortal(session.portalDomain) }
                }
                .font(.caption)
            } else {
                Text("Authenticated")
                    .font(.caption)
                    .foregroundStyle(.green)
            }
        }
    }

    private var awsProfileGroupsSection: some View {
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
            .padding(.horizontal)
        }
    }

    private var awsProfilesListSection: some View {
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
        .padding(.horizontal)
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

    // MARK: - GCP Tab

    @State private var showNewConfigForm = false
    @State private var newConfigName = ""
    @State private var newConfigAccount = ""
    @State private var newConfigProject = ""
    @State private var newConfigRegion = ""

    private var gcpTab: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                gcpAuthenticatedAccountsSection
                gcpProjectGroupsSection
                gcpConfigurationsSection
                Spacer()
            }
            .padding(.top)
        }
    }

    private var gcpAuthenticatedAccountsSection: some View {
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
        .padding(.horizontal)
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
            } else {
                Button {
                    Task { await viewModel.revokeGCPAccount(account.account) }
                } label: {
                    Image(systemName: "xmark.circle")
                        .foregroundStyle(.red.opacity(0.6))
                }
                .buttonStyle(.borderless)
                .help("Revoke \(account.account)")
            }
        }
    }

    private var gcpProjectGroupsSection: some View {
        ForEach(viewModel.gcpAccounts) { account in
            GroupBox(viewModel.redact(account.account, as: .email)) {
                VStack(alignment: .leading, spacing: 4) {
                    gcpAccountProjectsContent(account: account)
                }
                .padding(8)
            }
            .padding(.horizontal)
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

    private var gcpConfigurationsSection: some View {
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
        .padding(.horizontal)
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
            .help("Delete configuration")
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

    // MARK: - General Tab

    @State private var shellInstallStatus: ShellInstallStatus = .unknown
    @State private var newCustomTagName = ""

    private var generalTab: some View {
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
                LabeledContent("Version", value: "1.0.0")
                LabeledContent("Config Path", value: "~/Library/Application Support/Saddlebag/")
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
                            gcp: viewModel.userConfig.showGCPProjectInMenuBar
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
                            gcp: viewModel.userConfig.showGCPProjectInMenuBar
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
                            gcp: newVal
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

    private var shellIntegrationSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Add this to your ~/.zshrc to use the active profile in new terminals:")
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

                if shellInstallStatus == .error {
                    Text("Could not write to ~/.zshrc")
                        .font(.caption)
                        .foregroundStyle(.red)
                }
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
                    .help("Delete tag")
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

    private static let zshrcSnippet = """
    # Saddlebag: load active AWS profile
    if [ -f "$HOME/Library/Application Support/Saddlebag/config.json" ]; then
        export AWS_PROFILE=$(cat "$HOME/Library/Application Support/Saddlebag/config.json" | python3 -c "import sys,json; c=json.load(sys.stdin); print(c.get('activeAWSProfile',''))" 2>/dev/null)
    fi
    """

    private static let zshrcMarker = "# Saddlebag: load active AWS profile"

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

        // Check if already installed
        if let content = try? String(contentsOfFile: zshrcPath, encoding: .utf8),
           content.contains(Self.zshrcMarker) {
            shellInstallStatus = .alreadyInstalled
            return
        }

        // Append snippet
        do {
            let handle = try FileHandle(forWritingTo: URL(fileURLWithPath: zshrcPath))
            handle.seekToEndOfFile()
            let snippet = "\n\n" + Self.zshrcSnippet + "\n"
            handle.write(snippet.data(using: .utf8)!)
            handle.closeFile()
            shellInstallStatus = .installed
        } catch {
            // File may not exist — create it
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

enum ShellInstallStatus {
    case unknown
    case notInstalled
    case alreadyInstalled
    case installed
    case error
}

// MARK: - AWS Profile Settings Row

struct AWSProfileSettingsRow: View {
    let profile: AWSProfile
    let userConfig: UserConfig
    let onSetLabel: (String?) -> Void
    let onSetTag: (ProfileTag?) -> Void
    let onToggleFavorite: () -> Void

    @State private var editingLabel: String = ""
    @State private var isEditingLabel = false

    var body: some View {
        HStack {
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    if isEditingLabel {
                        TextField("Label", text: $editingLabel)
                            .textFieldStyle(.roundedBorder)
                            .frame(width: 160)
                            .onSubmit {
                                onSetLabel(editingLabel.isEmpty ? nil : editingLabel)
                                isEditingLabel = false
                            }
                    } else {
                        Text({
                            let label = userConfig.displayLabel(for: profile.name)
                            return userConfig.screenshotMode ? Obfuscator.redact(label, as: .generic) : label
                        }())
                            .font(.system(.body, weight: .medium))
                    }

                    if userConfig.isFavorite(profile.name) {
                        Image(systemName: "star.fill")
                            .font(.system(size: 10))
                            .foregroundStyle(.yellow)
                    }
                }

                Text({
                    let name = profile.name
                    let acctId = profile.accountId
                    if userConfig.screenshotMode {
                        return "\(Obfuscator.redact(name, as: .generic)) · \(profile.roleLabel) · \(Obfuscator.redact(acctId, as: .accountId))"
                    }
                    return "\(name) · \(profile.roleLabel) · \(acctId)"
                }())
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .privacySensitive()
            }

            Spacer()

            // Tag picker
            Picker("", selection: Binding(
                get: { userConfig.profileTags[profile.name] },
                set: { onSetTag($0) }
            )) {
                Text("No Tag").tag(nil as ProfileTag?)
                Divider()
                ForEach(userConfig.allTags, id: \.self) { tag in
                    Text("\(tag.emoji) \(tag.label)").tag(tag as ProfileTag?)
                }
            }
            .frame(width: 150)

            // Favorite toggle
            Button {
                onToggleFavorite()
            } label: {
                Image(systemName: userConfig.isFavorite(profile.name) ? "star.fill" : "star")
                    .foregroundStyle(userConfig.isFavorite(profile.name) ? .yellow : .secondary)
            }
            .buttonStyle(.borderless)
            .help("Toggle favorite")

            // Edit label
            Button {
                editingLabel = userConfig.profileLabels[profile.name] ?? ""
                isEditingLabel.toggle()
            } label: {
                Image(systemName: "pencil")
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.borderless)
            .help("Edit label")
        }
        .padding(.vertical, 2)
    }
}

// MARK: - GCP Project Settings Row

struct GCPProjectSettingsRow: View {
    let project: GCPProject
    let userConfig: UserConfig
    let onSetLabel: (String?) -> Void
    let onSetTag: (ProfileTag?) -> Void
    let onToggleFavorite: () -> Void

    @State private var editingLabel: String = ""
    @State private var isEditingLabel = false

    private var projectKey: String { "gcp-project:\(project.projectId)" }

    var body: some View {
        HStack {
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    if isEditingLabel {
                        TextField("Label", text: $editingLabel)
                            .textFieldStyle(.roundedBorder)
                            .frame(width: 160)
                            .onSubmit {
                                onSetLabel(editingLabel.isEmpty ? nil : editingLabel)
                                isEditingLabel = false
                            }
                    } else {
                        Text({
                            let label = userConfig.profileLabels[projectKey] ?? project.name
                            return userConfig.screenshotMode ? Obfuscator.redact(label, as: .generic) : label
                        }())
                            .font(.system(.body, weight: .medium))
                    }

                    if userConfig.isFavorite(projectKey) {
                        Image(systemName: "star.fill")
                            .font(.system(size: 10))
                            .foregroundStyle(.yellow)
                    }
                }

                Text(userConfig.screenshotMode ? Obfuscator.redact(project.projectId, as: .projectId) : project.projectId)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .privacySensitive()
            }

            Spacer()

            // Tag picker
            Picker("", selection: Binding(
                get: { userConfig.profileTags[projectKey] },
                set: { onSetTag($0) }
            )) {
                Text("No Tag").tag(nil as ProfileTag?)
                Divider()
                ForEach(userConfig.allTags, id: \.self) { tag in
                    Text("\(tag.emoji) \(tag.label)").tag(tag as ProfileTag?)
                }
            }
            .frame(width: 150)

            // Favorite toggle
            Button { onToggleFavorite() } label: {
                Image(systemName: userConfig.isFavorite(projectKey) ? "star.fill" : "star")
                    .foregroundStyle(userConfig.isFavorite(projectKey) ? .yellow : .secondary)
            }
            .buttonStyle(.borderless)
            .help("Toggle favorite")

            // Edit label
            Button {
                editingLabel = userConfig.profileLabels[projectKey] ?? ""
                isEditingLabel.toggle()
            } label: {
                Image(systemName: "pencil")
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.borderless)
            .help("Edit label")
        }
        .padding(.vertical, 2)
    }
}
