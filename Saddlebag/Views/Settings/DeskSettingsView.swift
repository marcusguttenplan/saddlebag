import SwiftUI
import SaddlebagShared

/// Desk configuration settings — create, edit, and manage desk context bundles
/// Uses the same ScrollView + GroupBox layout as AWS/GCP tabs for consistency
struct DeskSettingsView: View {
    @Bindable var viewModel: AccountsViewModel

    @State private var editingDeskId: String?
    @State private var isCreatingNew = false

    // Form state
    @State private var formName = ""
    @State private var formAWSProfile: String?
    @State private var formGCPConfig: String?
    @State private var formGitEmail = ""
    @State private var formGitName = ""
    @State private var formSSHKey = ""
    @State private var formGitHubKey = ""
    @State private var formWorkingDir = ""

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                // Desk list
                GroupBox("Desks") {
                    VStack(alignment: .leading, spacing: 8) {
                        ForEach(viewModel.desks) { desk in
                            deskRow(desk)
                        }

                        Divider()

                        if isCreatingNew {
                            deskForm(existingId: nil)
                        } else {
                            Button("New Desk…") {
                                isCreatingNew = true
                                editingDeskId = nil
                                resetForm()
                            }
                        }
                    }
                    .padding(8)
                }

                // Selected desk editor
                if let deskId = editingDeskId,
                   let desk = viewModel.desks.first(where: { $0.id == deskId }) {
                    deskForm(existingId: desk.id)
                }

                Spacer()
            }
            .padding()
        }
    }

    // MARK: - Desk Row

    private func deskRow(_ desk: Desk) -> some View {
        let isActive = desk.id == viewModel.activeDesk?.id
        let isEditing = editingDeskId == desk.id

        return HStack {
            Image(systemName: isActive ? "desktopcomputer.and.arrow.down" : "desktopcomputer")
                .foregroundStyle(isActive ? .green : .secondary)
            VStack(alignment: .leading, spacing: 1) {
                Text(desk.name)
                    .font(.system(.body, weight: .medium))
                Text(deskSubtitle(desk))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer()

            if isActive {
                HStack(spacing: 4) {
                    Image(systemName: "checkmark.circle.fill")
                        .foregroundStyle(.green)
                        .font(.system(size: 10))
                    Text("Active")
                }
                .font(.caption)
                .foregroundStyle(.secondary)
            } else {
                Button("Switch") {
                    Task { await viewModel.switchDesk(desk) }
                }
                .font(.caption)
            }

            Button {
                if isEditing {
                    editingDeskId = nil
                } else {
                    editingDeskId = desk.id
                    isCreatingNew = false
                    loadDeskIntoForm(desk)
                }
            } label: {
                Image(systemName: isEditing ? "xmark.circle" : "pencil")
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.borderless)
            .help(isEditing ? "Close editor" : "Edit desk")

            Button {
                Task {
                    await viewModel.deleteDesk(desk)
                    if editingDeskId == desk.id { editingDeskId = nil }
                }
            } label: {
                Image(systemName: "trash")
                    .foregroundStyle(.red.opacity(0.6))
            }
            .buttonStyle(.borderless)
            .help("Delete desk")
        }
    }

    private func deskSubtitle(_ desk: Desk) -> String {
        var parts: [String] = []
        if let aws = desk.awsProfile { parts.append(aws) }
        if let gcp = desk.gcpConfig { parts.append(gcp) }
        if let email = desk.gitEmail { parts.append(email) }
        return parts.isEmpty ? "No configuration" : parts.joined(separator: " · ")
    }

    // MARK: - Desk Form (Create or Edit)

    @ViewBuilder
    private func deskForm(existingId: String?) -> some View {
        let isNew = existingId == nil

        GroupBox(isNew ? "New Desk" : "Edit: \(formName)") {
            VStack(alignment: .leading, spacing: 12) {
                // Name + working dir
                VStack(alignment: .leading, spacing: 4) {
                    Text("Identity")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    HStack(spacing: 8) {
                        TextField("Name", text: $formName)
                            .textFieldStyle(.roundedBorder)
                            .frame(width: 180)
                    }
                    HStack(spacing: 8) {
                        Text("Workspace")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .frame(width: 70, alignment: .leading)
                        Text(formWorkingDir.isEmpty ? "No folder selected" : formWorkingDir)
                            .font(.system(.caption, design: .monospaced))
                            .foregroundStyle(formWorkingDir.isEmpty ? .tertiary : .primary)
                            .lineLimit(1)
                            .truncationMode(.middle)
                        Spacer()
                        Button("Browse…") {
                            let panel = NSOpenPanel()
                            panel.canChooseDirectories = true
                            panel.canChooseFiles = false
                            panel.allowsMultipleSelection = false
                            panel.prompt = "Select Workspace"
                            if panel.runModal() == .OK, let url = panel.url {
                                formWorkingDir = url.path
                            }
                        }
                        .font(.caption)
                        if !formWorkingDir.isEmpty {
                            Button {
                                formWorkingDir = ""
                            } label: {
                                Image(systemName: "xmark.circle")
                                    .foregroundStyle(.secondary)
                            }
                            .buttonStyle(.borderless)
                        }
                    }
                }

                Divider()

                // Cloud profiles
                VStack(alignment: .leading, spacing: 4) {
                    Text("Cloud Profiles")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    HStack(spacing: 16) {
                        Picker("AWS", selection: $formAWSProfile) {
                            Text("None").tag(nil as String?)
                            Divider()
                            ForEach(viewModel.awsProfiles) { profile in
                                Text(profile.name).tag(profile.name as String?)
                            }
                        }
                        .frame(width: 220)

                        Picker("GCP", selection: $formGCPConfig) {
                            Text("None").tag(nil as String?)
                            Divider()
                            ForEach(viewModel.gcpConfigurations) { config in
                                Text(config.name).tag(config.name as String?)
                            }
                        }
                        .frame(width: 220)
                    }
                }

                Divider()

                // Git identity
                VStack(alignment: .leading, spacing: 4) {
                    Text("Git Identity")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    HStack(spacing: 8) {
                        TextField("Email", text: $formGitEmail, prompt: Text("user@example.com"))
                            .textFieldStyle(.roundedBorder)
                        TextField("Name", text: $formGitName, prompt: Text("First Last"))
                            .textFieldStyle(.roundedBorder)
                    }
                }

                Divider()

                // SSH keys
                VStack(alignment: .leading, spacing: 4) {
                    Text("SSH Keys")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    HStack(spacing: 8) {
                        TextField("Device Key", text: $formSSHKey, prompt: Text("~/.ssh/id_ed25519"))
                            .textFieldStyle(.roundedBorder)
                        Button("Browse…") {
                            browseForSSHKey(binding: $formSSHKey)
                        }
                        .font(.caption)
                    }
                    HStack(spacing: 8) {
                        TextField("GitHub Key", text: $formGitHubKey, prompt: Text("~/.ssh/id_github"))
                            .textFieldStyle(.roundedBorder)
                        Button("Browse…") {
                            browseForSSHKey(binding: $formGitHubKey)
                        }
                        .font(.caption)
                    }

                    // Key existence check
                    if !formSSHKey.isEmpty || !formGitHubKey.isEmpty {
                        HStack(spacing: 12) {
                            if !formSSHKey.isEmpty {
                                keyStatusBadge(path: formSSHKey, label: "Device")
                            }
                            if !formGitHubKey.isEmpty {
                                keyStatusBadge(path: formGitHubKey, label: "GitHub")
                            }
                        }
                    }
                }

                Divider()

                // Actions
                HStack {
                    if isNew {
                        Button("Create Desk") {
                            Task { await createDesk() }
                        }
                        .disabled(formName.isEmpty)

                        Button("Cancel") {
                            isCreatingNew = false
                            resetForm()
                        }
                    } else {
                        Button("Save Changes") {
                            Task { await saveDesk(existingId: existingId!) }
                        }
                        .disabled(formName.isEmpty)

                        Button("Done") {
                            editingDeskId = nil
                        }
                    }
                }
            }
            .padding(8)
        }
    }

    private func keyStatusBadge(path: String, label: String) -> some View {
        let expanded = (path as NSString).expandingTildeInPath
        let exists = FileManager.default.fileExists(atPath: expanded)
        return HStack(spacing: 4) {
            Image(systemName: exists ? "checkmark.circle.fill" : "exclamationmark.triangle.fill")
                .font(.system(size: 10))
                .foregroundStyle(exists ? .green : .orange)
            Text("\(label): \(exists ? "found" : "not found")")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }

    // MARK: - Form Helpers

    private func loadDeskIntoForm(_ desk: Desk) {
        formName = desk.name
        formAWSProfile = desk.awsProfile
        formGCPConfig = desk.gcpConfig
        formGitEmail = desk.gitEmail ?? ""
        formGitName = desk.gitName ?? ""
        formSSHKey = desk.sshKey ?? ""
        formGitHubKey = desk.envVars["GITHUB_SSH_KEY"] ?? ""
        formWorkingDir = desk.workingDir ?? ""
    }

    private func resetForm() {
        formName = ""
        formAWSProfile = nil
        formGCPConfig = nil
        formGitEmail = ""
        formGitName = ""
        formSSHKey = ""
        formGitHubKey = ""
        formWorkingDir = ""
    }

    private func buildDesk(id: String) -> Desk {
        var envVars: [String: String] = [:]
        if !formGitHubKey.isEmpty {
            envVars["GITHUB_SSH_KEY"] = formGitHubKey
        }

        return Desk(
            id: id,
            name: formName,
            awsProfile: formAWSProfile,
            gcpConfig: formGCPConfig,
            gitEmail: formGitEmail.isEmpty ? nil : formGitEmail,
            gitName: formGitName.isEmpty ? nil : formGitName,
            sshKey: formSSHKey.isEmpty ? nil : formSSHKey,
            workingDir: formWorkingDir.isEmpty ? nil : formWorkingDir,
            envVars: envVars
        )
    }

    private func createDesk() async {
        let id = formName
            .lowercased()
            .replacingOccurrences(of: " ", with: "-")
            .filter { $0.isLetter || $0.isNumber || $0 == "-" }

        let desk = buildDesk(id: id)
        await viewModel.saveDesk(desk)
        isCreatingNew = false
        editingDeskId = desk.id
    }

    private func saveDesk(existingId: String) async {
        let desk = buildDesk(id: existingId)
        await viewModel.saveDesk(desk)
    }

    private func browseForSSHKey(binding: Binding<String>) {
        let panel = NSOpenPanel()
        panel.canChooseFiles = true
        panel.canChooseDirectories = false
        panel.allowsMultipleSelection = false
        panel.directoryURL = URL(fileURLWithPath: NSHomeDirectory() + "/.ssh")
        panel.showsHiddenFiles = true
        panel.title = "Select SSH Key"

        if panel.runModal() == .OK, let url = panel.url {
            let path = url.path
            let home = NSHomeDirectory()
            if path.hasPrefix(home) {
                binding.wrappedValue = "~" + path.dropFirst(home.count)
            } else {
                binding.wrappedValue = path
            }
        }
    }
}
