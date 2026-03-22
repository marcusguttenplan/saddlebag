import SwiftUI
import SaddlebagShared

// MARK: - Sort Mode

enum SectionSortMode: String, CaseIterable {
    case alphabetical
    case byTag

    var label: String {
        switch self {
        case .alphabetical: return "A→Z"
        case .byTag: return "Tag"
        }
    }

    var icon: String {
        switch self {
        case .alphabetical: return "textformat.abc"
        case .byTag: return "tag"
        }
    }
}

/// Main menubar dropdown panel
struct MenuBarView: View {
    @Bindable var viewModel: AccountsViewModel

    // MARK: - Expand/collapse state
    @State private var awsPlatformExpanded = true
    @State private var gcpPlatformExpanded = true
    @State private var expandedAWSPortals: Set<String> = []
    @State private var expandedGCPAccounts: Set<String> = []
    @State private var sortMode: SectionSortMode = .alphabetical
    @State private var hasInitializedExpansion = false

    var body: some View {
        VStack(spacing: 0) {
            // Header
            header

            Divider()

            // Scrollable content
            ScrollView {
                VStack(alignment: .leading, spacing: 8) {
                    // High-level overview
                    overviewSection

                    Divider()
                        .padding(.horizontal, 12)

                    // Favorites section
                    if !viewModel.favoriteProfiles.isEmpty {
                        favoritesSection
                        Divider()
                            .padding(.horizontal, 12)
                    }

                    // Accounts header + expandable sections
                    sectionHeader("Accounts", icon: "person.2")

                    // AWS platform section
                    awsPlatformSection

                    // GCP platform section
                    gcpPlatformSection
                }
                .padding(.vertical, 8)
            }

            Divider()

            // Footer actions
            footer
        }
        .task {
            await viewModel.start()
        }
        .onChange(of: viewModel.groupedAWSProfiles.count) {
            initializeExpansionIfNeeded()
        }
        .onChange(of: viewModel.gcpAccounts.count) {
            initializeExpansionIfNeeded()
        }
    }

    /// Expand all sections by default on first data load
    private func initializeExpansionIfNeeded() {
        guard !hasInitializedExpansion else { return }
        if !viewModel.groupedAWSProfiles.isEmpty || !viewModel.gcpAccounts.isEmpty {
            hasInitializedExpansion = true
            expandedAWSPortals = Set(viewModel.groupedAWSProfiles.map(\.portal))
            expandedGCPAccounts = Set(viewModel.gcpAccounts.map(\.account))
        }
    }

    // MARK: - Header

    private var header: some View {
        HStack {
            Text("Saddlebag")
                .font(.system(.headline, design: .rounded))

            Spacer()

            // Sort toggle
            sortToggle

            if viewModel.isLoading {
                ProgressView()
                    .controlSize(.small)
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
    }

    private var sortToggle: some View {
        HStack(spacing: 2) {
            ForEach(SectionSortMode.allCases, id: \.self) { mode in
                Button {
                    withAnimation(.easeInOut(duration: 0.15)) {
                        sortMode = mode
                    }
                } label: {
                    Image(systemName: mode.icon)
                        .font(.system(size: 10))
                        .frame(width: 22, height: 18)
                        .background(sortMode == mode ? Color.accentColor.opacity(0.2) : Color.clear)
                        .clipShape(RoundedRectangle(cornerRadius: 4))
                }
                .buttonStyle(.borderless)
                .help("Sort \(mode.label)")
            }
        }
        .padding(.trailing, 4)
    }

    // MARK: - Favorites

    private var favoritesSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            sectionHeader("Favorites", icon: "star.fill")

            ForEach(viewModel.favoriteProfiles) { profile in
                let tokenCache = resolveTokenCache(for: profile)
                ProfileRowView(
                    profile: profile,
                    userConfig: viewModel.userConfig,
                    tokenCache: tokenCache,
                    onTap: { Task { await handleAWSTap(profile) } }
                )
                .contextMenu { awsContextMenu(for: profile) }
            }
        }
    }

    // MARK: - Overview

    private var overviewSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            // Active desk indicator
            deskIndicator

            // Active AWS profile
            if let activeProfileName = viewModel.userConfig.activeAWSProfile,
               let activeProfile = viewModel.awsProfiles.first(where: { $0.name == activeProfileName }) {
                let tokenCache = resolveTokenCache(for: activeProfile)
                HStack(spacing: 8) {
                    Image(systemName: "server.rack")
                        .font(.system(size: 10))
                        .foregroundStyle(.secondary)
                        .frame(width: 16)
                    VStack(alignment: .leading, spacing: 1) {
                        Text(viewModel.redact(viewModel.userConfig.displayLabel(for: activeProfile.name), as: .generic))
                            .font(.system(.body, weight: .medium))
                        HStack(spacing: 6) {
                            Text(activeProfile.roleLabel)
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            if let remaining = tokenCache?.timeRemainingFormatted {
                                HStack(spacing: 2) {
                                    Image(systemName: "clock")
                                        .font(.system(size: 9))
                                    Text(remaining)
                                        .font(.system(.caption, design: .monospaced))
                                }
                                .foregroundStyle(tokenCache?.expiryStatus == .expiringSoon ? .orange : .secondary)
                            }
                        }
                    }
                    Spacer()
                    Circle()
                        .fill(overviewDotColor(for: tokenCache))
                        .frame(width: 8, height: 8)
                }
                .padding(.horizontal, 16)
                .contextMenu { awsContextMenu(for: activeProfile) }
            } else {
                HStack(spacing: 8) {
                    Image(systemName: "server.rack")
                        .font(.system(size: 10))
                        .foregroundStyle(.secondary)
                        .frame(width: 16)
                    Text("No active AWS profile")
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                }
                .padding(.horizontal, 16)
            }

            // Active GCP project
            if let activeConfig = viewModel.gcpConfigurations.first(where: { $0.isActive }) {
                HStack(spacing: 8) {
                    Image(systemName: "globe")
                        .font(.system(size: 10))
                        .foregroundStyle(.secondary)
                        .frame(width: 16)
                    VStack(alignment: .leading, spacing: 1) {
                        Text(viewModel.redact(activeConfig.displayName, as: .projectId))
                            .font(.system(.body, weight: .medium))
                        Text(viewModel.redact(activeConfig.account ?? "", as: .email))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                    Circle()
                        .fill(Color.green)
                        .frame(width: 8, height: 8)
                }
                .padding(.horizontal, 16)
                .contextMenu {
                    Button("Copy Project ID") {
                        NSPasteboard.general.clearContents()
                        NSPasteboard.general.setString(activeConfig.project ?? "", forType: .string)
                    }
                    Button("Copy Env Vars") {
                        viewModel.copyEnvVars(for: activeConfig)
                    }
                    Button("Open Console") {
                        viewModel.openGCPConsole(for: activeConfig)
                    }
                    Divider()
                    if let account = activeConfig.account {
                        Button("gcloud auth login") {
                            Task { await viewModel.gcloudAuthLogin(account: account) }
                        }
                    }
                    if let project = activeConfig.project {
                        Button("gcloud auth application-default login") {
                            Task { await viewModel.gcloudApplicationDefaultLogin(project: project) }
                        }
                    }
                }
            } else {
                HStack(spacing: 8) {
                    Image(systemName: "globe")
                        .font(.system(size: 10))
                        .foregroundStyle(.secondary)
                        .frame(width: 16)
                    Text("No active GCP project")
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                }
                .padding(.horizontal, 16)
            }
        }
    }

    // MARK: - Desk Indicator

    private var deskIndicator: some View {
        HStack(spacing: 8) {
            Image(systemName: "desktopcomputer")
                .font(.system(size: 10))
                .foregroundStyle(.secondary)
                .frame(width: 16)

            if let desk = viewModel.activeDesk {
                VStack(alignment: .leading, spacing: 1) {
                    Text(desk.name)
                        .font(.system(.body, weight: .semibold))
                    HStack(spacing: 6) {
                        if let aws = desk.awsProfile {
                            Text(viewModel.redact(aws, as: .generic))
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                        if let gcp = desk.gcpConfig {
                            Text(viewModel.redact(gcp, as: .generic))
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                }
            } else {
                Text("No active desk")
                    .font(.caption)
                    .foregroundStyle(.tertiary)
            }

            Spacer()

            if viewModel.activeDesk != nil {
                Circle()
                    .fill(.green)
                    .frame(width: 8, height: 8)
            }

            if !viewModel.desks.isEmpty {
                Menu {
                    ForEach(viewModel.desks) { desk in
                        Button {
                            Task { await viewModel.switchDesk(desk) }
                        } label: {
                            HStack {
                                Text(desk.name)
                                if desk.id == viewModel.activeDesk?.id {
                                    Image(systemName: "checkmark")
                                }
                            }
                        }
                    }
                } label: {
                    Image(systemName: "arrow.triangle.swap")
                        .font(.system(size: 10))
                        .foregroundStyle(.secondary)
                }
                .menuStyle(.borderlessButton)
                .frame(width: 20)
            }
        }
        .padding(.horizontal, 16)
    }

    private func overviewDotColor(for tokenCache: SSOTokenCache?) -> Color {
        guard let cache = tokenCache else { return .gray.opacity(0.5) }
        switch cache.expiryStatus {
        case .valid: return .green
        case .expiringSoon: return .orange
        case .expired: return .gray
        case .unknown: return .gray.opacity(0.5)
        }
    }

    // MARK: - AWS Platform Section

    private var awsPlatformSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            if !viewModel.groupedAWSProfiles.isEmpty {
                // Platform header
                platformHeader(
                    title: "AWS",
                    icon: "server.rack",
                    isExpanded: awsPlatformExpanded,
                    count: viewModel.awsProfiles.count,
                    onToggle: { withAnimation(.easeInOut(duration: 0.15)) { awsPlatformExpanded.toggle() } }
                )

                if awsPlatformExpanded {
                    ForEach(viewModel.groupedAWSProfiles, id: \.portal) { group in
                        awsPortalSection(group: group)
                    }
                }
            }
        }
    }

    private func awsPortalSection(group: (portal: String, portalDisplayName: String, profiles: [AWSProfile])) -> some View {
        let portalHasActiveProfile = group.profiles.contains { profile in
            let isActive = viewModel.userConfig.activeAWSProfile == profile.name
            if !isActive { return false }
            let cache = resolveTokenCache(for: profile)
            return cache?.expiryStatus == .valid || cache?.expiryStatus == .expiringSoon
        }

        return VStack(alignment: .leading, spacing: 2) {
            // Portal sub-header
            HStack {
                Button {
                    withAnimation(.easeInOut(duration: 0.15)) {
                        if expandedAWSPortals.contains(group.portal) {
                            expandedAWSPortals.remove(group.portal)
                        } else {
                            expandedAWSPortals.insert(group.portal)
                        }
                    }
                } label: {
                    HStack(spacing: 5) {
                        Image(systemName: expandedAWSPortals.contains(group.portal) ? "chevron.down" : "chevron.right")
                            .font(.system(size: 8, weight: .bold))
                            .frame(width: 10)
                        Text(viewModel.redact(group.portalDisplayName, as: .generic))
                            .font(.system(size: 11, weight: .medium))
                        Text("(\(group.profiles.count))")
                            .font(.system(size: 10))
                            .foregroundStyle(.tertiary)
                    }
                    .foregroundStyle(.secondary)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.borderless)
                .padding(.leading, 28)

                Spacer()

                if portalHasActiveProfile {
                    Text("active")
                        .font(.system(size: 9, weight: .medium))
                        .foregroundStyle(.green)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 1)
                        .background(.green.opacity(0.15))
                        .clipShape(RoundedRectangle(cornerRadius: 3))
                } else {
                    Button {
                        Task { await viewModel.loginPortal(group.portal) }
                    } label: {
                        HStack(spacing: 3) {
                            Image(systemName: "arrow.right.circle")
                                .font(.system(size: 10))
                            Text("Login")
                                .font(.system(size: 10, weight: .medium))
                        }
                        .foregroundStyle(.orange)
                    }
                    .buttonStyle(.borderless)
                    .help("Authenticate all \(group.portalDisplayName) profiles")
                }
                Spacer().frame(width: 16)
            }
            .padding(.top, 2)

            if expandedAWSPortals.contains(group.portal) {
                ForEach(sortedAWSProfiles(group.profiles)) { profile in
                    let tokenCache = resolveTokenCache(for: profile)
                    ProfileRowView(
                        profile: profile,
                        userConfig: viewModel.userConfig,
                        tokenCache: tokenCache,
                        onTap: { Task { await handleAWSTap(profile) } }
                    )
                    .contextMenu { awsContextMenu(for: profile) }
                }
            }
        }
    }

    // MARK: - GCP Platform Section

    private var gcpPlatformSection: some View {
        VStack(alignment: .leading, spacing: 4) {
            if !viewModel.gcpAccounts.isEmpty {
                Divider()
                    .padding(.horizontal, 12)

                // Platform header
                platformHeader(
                    title: "GCP",
                    icon: "globe",
                    isExpanded: gcpPlatformExpanded,
                    count: viewModel.gcpProjectsByAccount.values.reduce(0) { $0 + $1.count },
                    onToggle: { withAnimation(.easeInOut(duration: 0.15)) { gcpPlatformExpanded.toggle() } }
                )

                if gcpPlatformExpanded {
                    ForEach(viewModel.gcpAccounts) { account in
                        gcpAccountSection(account: account)
                    }
                }
            }
        }
    }

    private func gcpAccountSection(account: GCPAccount) -> some View {
        let needsAuth = viewModel.gcpAccountAuthNeeded.contains(account.account)

        return VStack(alignment: .leading, spacing: 2) {
            // Account sub-header
            HStack {
                Button {
                    withAnimation(.easeInOut(duration: 0.15)) {
                        if expandedGCPAccounts.contains(account.account) {
                            expandedGCPAccounts.remove(account.account)
                        } else {
                            expandedGCPAccounts.insert(account.account)
                        }
                    }
                } label: {
                    HStack(spacing: 5) {
                        Image(systemName: expandedGCPAccounts.contains(account.account) ? "chevron.down" : "chevron.right")
                            .font(.system(size: 8, weight: .bold))
                            .frame(width: 10)
                        Text(viewModel.redact(accountDisplayName(account.account), as: .email))
                            .font(.system(size: 11, weight: .medium))
                        if let projects = viewModel.gcpProjectsByAccount[account.account] {
                            Text("(\(projects.count))")
                                .font(.system(size: 10))
                                .foregroundStyle(.tertiary)
                        }
                    }
                    .foregroundStyle(.secondary)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.borderless)
                .padding(.leading, 28)

                Spacer()

                if needsAuth {
                    Button {
                        Task { await viewModel.loginGCPAccount(account.account) }
                    } label: {
                        HStack(spacing: 3) {
                            Image(systemName: "arrow.right.circle")
                                .font(.system(size: 10))
                            Text("Login")
                                .font(.system(size: 10, weight: .medium))
                        }
                        .foregroundStyle(.orange)
                    }
                    .buttonStyle(.borderless)
                    .help("Re-authenticate \(account.account)")
                } else if account.isActive {
                    Text("active")
                        .font(.system(size: 9, weight: .medium))
                        .foregroundStyle(.green)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 1)
                        .background(.green.opacity(0.15))
                        .clipShape(RoundedRectangle(cornerRadius: 3))
                }
                Spacer().frame(width: 16)
            }
            .padding(.top, 2)

            if expandedGCPAccounts.contains(account.account) {
                // Projects for this account
                if viewModel.gcpAccountAuthNeeded.contains(account.account) {
                    HStack(spacing: 6) {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .font(.system(size: 10))
                            .foregroundStyle(.orange)
                        Text("Session expired — click Login to re-authenticate")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    .padding(.horizontal, 28)
                    .padding(.vertical, 4)
                } else if let projects = viewModel.gcpProjectsByAccount[account.account], !projects.isEmpty {
                    ForEach(sortedGCPProjects(projects)) { project in
                        gcpProjectRow(project: project, account: account.account)
                    }
                } else {
                    Text("No projects found")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .padding(.horizontal, 28)
                        .padding(.vertical, 4)
                }
            }
        }
    }

    private func accountDisplayName(_ email: String) -> String {
        // Use custom label if set, otherwise show full email
        let key = "gcp-account:\(email)"
        if let label = viewModel.userConfig.profileLabels[key] {
            return label
        }
        return email
    }

    private func gcpProjectRow(project: GCPProject, account: String) -> some View {
        let activeConfig = viewModel.gcpConfigurations.first(where: { $0.isActive })
        let isActive = activeConfig?.project == project.projectId && activeConfig?.account == account

        return Button {
            Task { await viewModel.setGCPProject(project.projectId, account: account) }
        } label: {
            HStack(spacing: 8) {
                Circle()
                    .fill(isActive ? Color.green : Color.gray.opacity(0.5))
                    .frame(width: 8, height: 8)

                VStack(alignment: .leading, spacing: 1) {
                    Text(viewModel.redact(viewModel.userConfig.profileLabels["gcp-project:\(project.projectId)"] ?? project.name, as: .generic))
                        .font(.system(.body, design: .default, weight: isActive ? .semibold : .regular))
                        .foregroundStyle(isActive ? .primary : .secondary)
                    Text(viewModel.redact(project.projectId, as: .projectId))
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                }

                Spacer()

                if let tag = viewModel.userConfig.profileTags["gcp-project:\(project.projectId)"] {
                    Text(tag.label)
                        .font(.system(size: 10, weight: .medium))
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(tag.color.opacity(0.15))
                        .foregroundStyle(tag.color)
                        .clipShape(Capsule())
                }
            }
            .padding(.vertical, 4)
            .padding(.horizontal, 8)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .contextMenu {
            Button("Copy Project ID") {
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(project.projectId, forType: .string)
            }
            Button("Copy Env Vars") {
                let env = [
                    "export GOOGLE_CLOUD_PROJECT=\(project.projectId)",
                    "export GCLOUD_PROJECT=\(project.projectId)",
                    "export GOOGLE_CLOUD_ACCOUNT=\(account)"
                ].joined(separator: "\n")
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(env, forType: .string)
            }
            Button("Open Console") {
                if let url = URL(string: "https://console.cloud.google.com/home/dashboard?project=\(project.projectId)") {
                    NSWorkspace.shared.open(url)
                }
            }

            Divider()

            Button("gcloud auth login") {
                Task { await viewModel.gcloudAuthLogin(account: account) }
            }
            Button("gcloud auth application-default login") {
                Task { await viewModel.gcloudApplicationDefaultLogin(project: project.projectId) }
            }
        }
    }

    // MARK: - Footer

    private var footer: some View {
        HStack(spacing: 12) {
            Button {
                Task { await viewModel.refresh() }
            } label: {
                Image(systemName: "arrow.clockwise")
                    .font(.system(size: 12))
            }
            .buttonStyle(.borderless)
            .help("Refresh")

            Spacer()

            Button("Open Saddlebag") {
                AppWindowManager.shared.open(viewModel: viewModel)
            }
            .buttonStyle(.borderless)
            .font(.system(size: 12, weight: .medium))

            Button {
                NSApplication.shared.terminate(nil)
            } label: {
                Image(systemName: "power")
                    .font(.system(size: 12))
            }
            .buttonStyle(.borderless)
            .help("Quit Saddlebag")
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 8)
    }

    // MARK: - Helpers

    /// Platform-level collapsible header (AWS / GCP)
    private func platformHeader(
        title: String,
        icon: String,
        isExpanded: Bool,
        count: Int,
        onToggle: @escaping () -> Void
    ) -> some View {
        Button(action: onToggle) {
            HStack(spacing: 6) {
                Image(systemName: isExpanded ? "chevron.down" : "chevron.right")
                    .font(.system(size: 9, weight: .bold))
                    .frame(width: 12)
                Image(systemName: icon)
                    .font(.system(size: 11))
                Text(title)
                    .font(.system(size: 12, weight: .semibold))
                Text("(\(count))")
                    .font(.system(size: 10))
                    .foregroundStyle(.tertiary)
            }
            .foregroundStyle(.secondary)
            .contentShape(Rectangle())
        }
        .buttonStyle(.borderless)
        .padding(.horizontal, 16)
        .padding(.top, 4)
    }

    private func sectionHeader(_ title: String, icon: String) -> some View {
        HStack(spacing: 6) {
            Image(systemName: icon)
                .font(.system(size: 10))
            Text(title)
                .font(.system(size: 11, weight: .semibold))
        }
        .foregroundStyle(.secondary)
        .padding(.horizontal, 16)
        .padding(.top, 4)
    }

    private func resolveTokenCache(for profile: AWSProfile) -> SSOTokenCache? {
        // Find the session for this profile, then look up its token
        guard let session = viewModel.ssoSessions.first(where: { $0.name == profile.ssoSessionName }) else {
            return nil
        }
        return viewModel.tokenStatuses[session.name]
    }

    private func handleAWSTap(_ profile: AWSProfile) async {
        let tokenCache = resolveTokenCache(for: profile)
        if tokenCache == nil || tokenCache?.isExpired == true {
            // Need to login
            await viewModel.loginAWS(profile: profile)
        } else {
            // Just set as active
            await viewModel.setActiveAWS(profile: profile)
        }
    }

    // MARK: - Sorting

    private func sortedAWSProfiles(_ profiles: [AWSProfile]) -> [AWSProfile] {
        switch sortMode {
        case .alphabetical:
            return profiles.sorted { a, b in
                viewModel.userConfig.displayLabel(for: a.name).localizedCaseInsensitiveCompare(
                    viewModel.userConfig.displayLabel(for: b.name)
                ) == .orderedAscending
            }
        case .byTag:
            return profiles.sorted { a, b in
                let tagA = viewModel.userConfig.profileTags[a.name]
                let tagB = viewModel.userConfig.profileTags[b.name]
                let orderA = tagA?.sortOrder ?? 999
                let orderB = tagB?.sortOrder ?? 999
                if orderA != orderB { return orderA < orderB }
                return viewModel.userConfig.displayLabel(for: a.name).localizedCaseInsensitiveCompare(
                    viewModel.userConfig.displayLabel(for: b.name)
                ) == .orderedAscending
            }
        }
    }

    private func sortedGCPProjects(_ projects: [GCPProject]) -> [GCPProject] {
        switch sortMode {
        case .alphabetical:
            return projects.sorted { a, b in
                let nameA = viewModel.userConfig.profileLabels["gcp-project:\(a.projectId)"] ?? a.name
                let nameB = viewModel.userConfig.profileLabels["gcp-project:\(b.projectId)"] ?? b.name
                return nameA.localizedCaseInsensitiveCompare(nameB) == .orderedAscending
            }
        case .byTag:
            return projects.sorted { a, b in
                let tagA = viewModel.userConfig.profileTags["gcp-project:\(a.projectId)"]
                let tagB = viewModel.userConfig.profileTags["gcp-project:\(b.projectId)"]
                let orderA = tagA?.sortOrder ?? 999
                let orderB = tagB?.sortOrder ?? 999
                if orderA != orderB { return orderA < orderB }
                let nameA = viewModel.userConfig.profileLabels["gcp-project:\(a.projectId)"] ?? a.name
                let nameB = viewModel.userConfig.profileLabels["gcp-project:\(b.projectId)"] ?? b.name
                return nameA.localizedCaseInsensitiveCompare(nameB) == .orderedAscending
            }
        }
    }

    // MARK: - Context Menus

    @ViewBuilder
    private func awsContextMenu(for profile: AWSProfile) -> some View {
        Button("Copy Env Vars") {
            viewModel.copyEnvVars(for: profile)
        }

        Button("Open AWS Console") {
            viewModel.openAWSConsole(for: profile)
        }

        Divider()

        Button("SSO Login") {
            Task { await viewModel.loginAWS(profile: profile) }
        }

        Button(viewModel.userConfig.isFavorite(profile.name) ? "Remove from Favorites" : "Add to Favorites") {
            Task { await viewModel.toggleFavorite(profile.name) }
        }

        Divider()

        Menu("Set Tag") {
            ForEach(viewModel.userConfig.allTags, id: \.self) { tag in
                Button("\(tag.emoji) \(tag.label)") {
                    Task { await viewModel.setTag(for: profile.name, tag: tag) }
                }
            }
            Divider()
            Button("Clear Tag") {
                Task { await viewModel.setTag(for: profile.name, tag: nil) }
            }
        }
    }

    @ViewBuilder
    private func gcpContextMenu(for config: GCPConfiguration) -> some View {
        Button("Copy Env Vars") {
            viewModel.copyEnvVars(for: config)
        }

        Button("Open GCP Console") {
            viewModel.openGCPConsole(for: config)
        }

        Button("Activate") {
            Task { await viewModel.switchGCP(to: config) }
        }
    }
}
