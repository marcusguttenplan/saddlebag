import SwiftUI
import SaddlebagShared

// MARK: - Shell Install Status

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
