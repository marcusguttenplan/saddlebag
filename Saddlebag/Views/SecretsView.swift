import SwiftUI
import UniformTypeIdentifiers
import SaddlebagShared

/// Full secrets management view — project selector, secret list, and actions
struct SecretsView: View {
    @Bindable var accountsViewModel: AccountsViewModel
    @State private var viewModel: SecretsViewModel
    @State private var showCreateSheet = false
    @State private var showEnvPathPicker = false
    @State private var revealedSecrets: Set<String> = []

    init(accountsViewModel: AccountsViewModel) {
        self.accountsViewModel = accountsViewModel
        self._viewModel = State(initialValue: SecretsViewModel(shell: ShellService()))
    }

    /// All GCP projects from all accounts, flattened
    private var allProjects: [GCPProject] {
        accountsViewModel.gcpProjectsByAccount.values
            .flatMap { $0 }
            .sorted { $0.name < $1.name }
    }

    var body: some View {
        VStack(spacing: 0) {
            headerBar
            Divider()
            if viewModel.isLoading && viewModel.secrets.isEmpty {
                loadingView
            } else if viewModel.selectedProject == nil {
                emptyProjectView
            } else if viewModel.secrets.isEmpty && !viewModel.isLoading {
                emptySecretsView
            } else {
                secretList
            }
            Divider()
            actionBar
        }
        .overlay(alignment: .bottom) {
            toastOverlay
        }
        .sheet(isPresented: $showCreateSheet) {
            CreateSecretSheet(viewModel: viewModel)
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

    // MARK: - Header

    private var headerBar: some View {
        VStack(spacing: 10) {
            HStack(spacing: 12) {
                // Project picker
                Picker("Project", selection: Binding(
                    get: { viewModel.selectedProject ?? "" },
                    set: { newValue in
                        if !newValue.isEmpty {
                            Task { await viewModel.switchProject(to: newValue) }
                        }
                    }
                )) {
                    Text("Select a project…").tag("")
                    Divider()
                    ForEach(allProjects) { project in
                        Text(accountsViewModel.redact(project.projectId, as: .projectId))
                            .tag(project.projectId)
                    }
                }
                .frame(minWidth: 200)

                Spacer()

                // Search field
                HStack(spacing: 4) {
                    Image(systemName: "magnifyingglass")
                        .foregroundStyle(.secondary)
                    TextField("Search secrets…", text: $viewModel.filterText)
                        .textFieldStyle(.plain)
                }
                .padding(.horizontal, 8)
                .padding(.vertical, 5)
                .background(.quaternary)
                .clipShape(RoundedRectangle(cornerRadius: 6))
                .frame(maxWidth: 200)
            }

            // Label filter bar
            HStack(spacing: 8) {
                filterPicker("Org", options: viewModel.availableOrgs, selection: $viewModel.filterOrg)
                filterPicker("Service", options: viewModel.availableServices, selection: $viewModel.filterService)
                filterPicker("Stage", options: viewModel.availableStages, selection: $viewModel.filterStage)

                Spacer()

                if viewModel.isLoading {
                    ProgressView()
                        .controlSize(.small)
                }

                Button {
                    Task { await viewModel.loadSecrets() }
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
                .help("Refresh secrets")
            }
        }
        .padding()
    }

    private func filterPicker(_ label: String, options: [String], selection: Binding<String?>) -> some View {
        Picker(label, selection: Binding(
            get: { selection.wrappedValue ?? "" },
            set: { selection.wrappedValue = $0.isEmpty ? nil : $0 }
        )) {
            Text("All").tag("")
            Divider()
            ForEach(options, id: \.self) { option in
                Text(option).tag(option)
            }
        }
        .frame(minWidth: 100)
    }

    // MARK: - Secret List

    private var secretList: some View {
        ScrollView {
            LazyVStack(spacing: 0) {
                // Select all header
                HStack(spacing: 8) {
                    Button {
                        if viewModel.allFilteredSelected {
                            viewModel.deselectAll()
                        } else {
                            viewModel.selectAll()
                        }
                    } label: {
                        Image(systemName: viewModel.allFilteredSelected ? "checkmark.square.fill" : "square")
                            .foregroundStyle(viewModel.allFilteredSelected ? Color.accentColor : .secondary)
                    }
                    .buttonStyle(.plain)

                    Text("Name")
                        .fontWeight(.medium)
                        .frame(minWidth: 160, alignment: .leading)
                    Text("Org")
                        .fontWeight(.medium)
                        .frame(width: 80, alignment: .leading)
                    Text("Service")
                        .fontWeight(.medium)
                        .frame(width: 80, alignment: .leading)
                    Text("Stage")
                        .fontWeight(.medium)
                        .frame(width: 70, alignment: .leading)
                    Text("Var")
                        .fontWeight(.medium)
                        .frame(minWidth: 100, alignment: .leading)
                    Spacer()
                }
                .font(.caption)
                .foregroundStyle(.secondary)
                .padding(.horizontal, 16)
                .padding(.vertical, 6)
                .background(.quaternary.opacity(0.5))

                Divider()

                ForEach(viewModel.filteredSecrets) { secret in
                    secretRow(secret)
                    Divider()
                }
            }
        }
    }

    private func secretRow(_ secret: Secret) -> some View {
        let isSelected = viewModel.selectedSecrets.contains(secret.id)
        let isRevealed = revealedSecrets.contains(secret.id)

        return VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 8) {
                // Checkbox
                Button {
                    viewModel.toggleSelection(secret.id)
                } label: {
                    Image(systemName: isSelected ? "checkmark.square.fill" : "square")
                        .foregroundStyle(isSelected ? Color.accentColor : .secondary)
                }
                .buttonStyle(.plain)

                // Name
                Text(secret.name)
                    .font(.system(.body, design: .monospaced))
                    .lineLimit(1)
                    .frame(minWidth: 160, alignment: .leading)

                // Labels
                labelBadge(secret.org, color: .blue)
                    .frame(width: 80, alignment: .leading)
                labelBadge(secret.service, color: .green)
                    .frame(width: 80, alignment: .leading)
                stageBadge(secret.stage)
                    .frame(width: 70, alignment: .leading)
                Text(secret.varName ?? "—")
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(secret.varName != nil ? .primary : .tertiary)
                    .frame(minWidth: 100, alignment: .leading)

                Spacer()

                // Reveal / Copy buttons
                Button {
                    if isRevealed {
                        revealedSecrets.remove(secret.id)
                    } else {
                        revealedSecrets.insert(secret.id)
                        Task { await viewModel.fetchValue(for: secret) }
                    }
                } label: {
                    Image(systemName: isRevealed ? "eye.slash" : "eye")
                        .font(.caption)
                }
                .buttonStyle(.borderless)
                .help(isRevealed ? "Hide value" : "Reveal value")

                Button {
                    Task { await viewModel.copyValue(for: secret) }
                } label: {
                    Image(systemName: "doc.on.doc")
                        .font(.caption)
                }
                .buttonStyle(.borderless)
                .help("Copy value")
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 8)
            .contentShape(Rectangle())
            .background(isSelected ? Color.accentColor.opacity(0.08) : Color.clear)

            // Revealed value
            if isRevealed, let value = viewModel.secretValues[secret.name] {
                HStack {
                    Text(value)
                        .font(.system(.caption, design: .monospaced))
                        .textSelection(.enabled)
                        .lineLimit(3)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 6)
                        .background(.quaternary)
                        .clipShape(RoundedRectangle(cornerRadius: 4))
                }
                .padding(.horizontal, 44)
                .padding(.bottom, 8)
            }
        }
    }

    @ViewBuilder
    private func labelBadge(_ value: String?, color: Color) -> some View {
        if let value {
            Text(value)
                .font(.caption2)
                .padding(.horizontal, 6)
                .padding(.vertical, 2)
                .background(color.opacity(0.15))
                .foregroundStyle(color)
                .clipShape(RoundedRectangle(cornerRadius: 4))
        } else {
            Text("—")
                .font(.caption2)
                .foregroundStyle(.tertiary)
        }
    }

    @ViewBuilder
    private func stageBadge(_ stage: String?) -> some View {
        if let stage {
            let color: Color = switch stage {
            case "prod": .red
            case "staging": .orange
            case "dev": .green
            default: .purple
            }
            Text(stage)
                .font(.caption2)
                .padding(.horizontal, 6)
                .padding(.vertical, 2)
                .background(color.opacity(0.15))
                .foregroundStyle(color)
                .clipShape(RoundedRectangle(cornerRadius: 4))
        } else {
            Text("—")
                .font(.caption2)
                .foregroundStyle(.tertiary)
        }
    }

    // MARK: - Action Bar

    private var actionBar: some View {
        HStack(spacing: 12) {
            let count = viewModel.selectedSecrets.count

            Text("\(count) selected")
                .font(.caption)
                .foregroundStyle(.secondary)

            Spacer()

            Button("Copy as .env") {
                Task { await viewModel.copySelectedAsEnv() }
            }
            .disabled(count == 0)
            .help("Copy selected secrets grouped by service/stage")

            Button("Copy as JSON") {
                Task { await viewModel.copySelectedAsJSON() }
            }
            .disabled(count == 0)

            Button("Generate .env files…") {
                showEnvPathPicker = true
            }
            .disabled(count == 0)
            .fileImporter(
                isPresented: $showEnvPathPicker,
                allowedContentTypes: [.folder],
                allowsMultipleSelection: false
            ) { result in
                if case .success(let urls) = result, let url = urls.first {
                    Task { await viewModel.generateEnvFiles(basePath: url.path) }
                }
            }

            Button {
                showCreateSheet = true
            } label: {
                Image(systemName: "plus")
            }
            .help("Create a new secret")
        }
        .padding()
    }

    // MARK: - Placeholder Views

    private var loadingView: some View {
        VStack(spacing: 12) {
            ProgressView()
            Text("Loading secrets…")
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var emptyProjectView: some View {
        VStack(spacing: 12) {
            Image(systemName: "key.fill")
                .font(.system(size: 40))
                .foregroundStyle(.secondary)
            Text("Select a GCP project")
                .font(.headline)
            Text("Choose a project from the dropdown to browse its secrets")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var emptySecretsView: some View {
        VStack(spacing: 12) {
            Image(systemName: "key.slash")
                .font(.system(size: 40))
                .foregroundStyle(.secondary)
            Text("No secrets found")
                .font(.headline)
            Text("This project has no secrets, or you don't have permission to list them")
                .font(.caption)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    // MARK: - Toast

    @ViewBuilder
    private var toastOverlay: some View {
        if let toast = viewModel.clipboardToast {
            Text(toast)
                .font(.caption)
                .padding(.horizontal, 12)
                .padding(.vertical, 8)
                .background(.ultraThinMaterial)
                .clipShape(RoundedRectangle(cornerRadius: 8))
                .shadow(radius: 4)
                .padding(.bottom, 60)
                .transition(.move(edge: .bottom).combined(with: .opacity))
                .onAppear {
                    DispatchQueue.main.asyncAfter(deadline: .now() + 2) {
                        withAnimation { viewModel.clipboardToast = nil }
                    }
                }
        }
    }
}

// MARK: - Create Secret Sheet

struct CreateSecretSheet: View {
    @Bindable var viewModel: SecretsViewModel
    @Environment(\.dismiss) private var dismiss

    @State private var name = ""
    @State private var value = ""
    @State private var org = ""
    @State private var service = ""
    @State private var stage = ""
    @State private var varName = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Create Secret")
                .font(.headline)

            GroupBox("Secret") {
                VStack(alignment: .leading, spacing: 8) {
                    TextField("Secret name", text: $name)
                        .textFieldStyle(.roundedBorder)
                        .font(.system(.body, design: .monospaced))

                    SecureField("Secret value", text: $value)
                        .textFieldStyle(.roundedBorder)
                }
                .padding(8)
            }

            GroupBox("Labels") {
                VStack(spacing: 8) {
                    HStack(spacing: 8) {
                        LabeledContent("Org") {
                            TextField("e.g. courseclear", text: $org)
                                .textFieldStyle(.roundedBorder)
                                .frame(width: 160)
                        }
                        LabeledContent("Service") {
                            TextField("e.g. api", text: $service)
                                .textFieldStyle(.roundedBorder)
                                .frame(width: 120)
                        }
                    }
                    HStack(spacing: 8) {
                        LabeledContent("Stage") {
                            TextField("e.g. prod", text: $stage)
                                .textFieldStyle(.roundedBorder)
                                .frame(width: 120)
                        }
                        LabeledContent("Var") {
                            TextField("e.g. DATABASE_URL", text: $varName)
                                .textFieldStyle(.roundedBorder)
                                .frame(width: 160)
                        }
                    }
                }
                .padding(8)
            }

            if let previewPath = envPathPreview {
                Text("Env output: \(previewPath)")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }

            HStack {
                Spacer()
                Button("Cancel") { dismiss() }
                    .keyboardShortcut(.cancelAction)
                Button("Create") {
                    Task {
                        await viewModel.createSecret(
                            name: name,
                            value: value,
                            org: org.isEmpty ? nil : org,
                            service: service.isEmpty ? nil : service,
                            stage: stage.isEmpty ? nil : stage,
                            varName: varName.isEmpty ? nil : varName
                        )
                        dismiss()
                    }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(name.isEmpty || value.isEmpty)
            }
        }
        .padding(20)
        .frame(minWidth: 480)
    }

    private var envPathPreview: String? {
        guard !service.isEmpty, !stage.isEmpty, !varName.isEmpty else { return nil }
        return "\(service)/.env.\(stage) → \(varName)=\"•••\""
    }
}
