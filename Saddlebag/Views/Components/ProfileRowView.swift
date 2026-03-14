import SwiftUI

/// Reusable row component for AWS profiles and GCP configurations
struct ProfileRowView: View {
    let title: String
    let subtitle: String
    let isActive: Bool
    let tag: ProfileTag?
    let isFavorite: Bool
    let tokenStatus: TokenExpiryStatus?
    let timeRemaining: String?
    let onTap: () -> Void

    var body: some View {
        Button(action: onTap) {
            HStack(spacing: 8) {
                // Status dot
                Circle()
                    .fill(statusColor)
                    .frame(width: 8, height: 8)

                // Main content
                VStack(alignment: .leading, spacing: 1) {
                    HStack(spacing: 6) {
                        Text(title)
                            .font(.system(.body, design: .default, weight: isActive ? .semibold : .regular))
                            .foregroundStyle(isActive ? .primary : .secondary)

                        if isFavorite {
                            Image(systemName: "star.fill")
                                .font(.system(size: 9))
                                .foregroundStyle(.yellow)
                        }
                    }

                    Text(subtitle)
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                }

                Spacer()

                // Tag badge
                if let tag {
                    Text(tag.label)
                        .font(.system(size: 10, weight: .medium))
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(tag.color.opacity(0.15))
                        .foregroundStyle(tag.color)
                        .clipShape(Capsule())
                }

                // Timer
                if let timeRemaining, tokenStatus != .expired {
                    HStack(spacing: 3) {
                        Image(systemName: "clock")
                            .font(.system(size: 10))
                        Text(timeRemaining)
                            .font(.system(.caption, design: .monospaced))
                    }
                    .foregroundStyle(timerColor)
                }
            }
            .padding(.vertical, 4)
            .padding(.horizontal, 8)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }

    private var statusColor: Color {
        if let tokenStatus {
            switch tokenStatus {
            case .valid: return .green
            case .expiringSoon: return .orange
            case .expired: return .gray
            case .unknown: return .gray
            }
        }
        return isActive ? .green : .gray.opacity(0.5)
    }

    private var timerColor: Color {
        switch tokenStatus {
        case .expiringSoon: return .orange
        case .expired: return .red
        default: return .secondary
        }
    }
}

// MARK: - Convenience Initializers

extension ProfileRowView {
    /// Create a row for an AWS profile
    init(
        profile: AWSProfile,
        userConfig: UserConfig,
        tokenCache: SSOTokenCache?,
        onTap: @escaping () -> Void
    ) {
        self.title = userConfig.displayLabel(for: profile.name)
        self.subtitle = "\(profile.roleLabel) · \(profile.accountId) · \(profile.region)"
        self.isActive = userConfig.activeAWSProfile == profile.name
        self.tag = userConfig.profileTags[profile.name]
        self.isFavorite = userConfig.isFavorite(profile.name)
        self.tokenStatus = tokenCache?.expiryStatus ?? .unknown
        self.timeRemaining = tokenCache?.timeRemainingFormatted
        self.onTap = onTap
    }

    /// Create a row for a GCP configuration
    init(
        gcpConfig: GCPConfiguration,
        userConfig: UserConfig,
        onTap: @escaping () -> Void
    ) {
        let configKey = "gcp:\(gcpConfig.name)"
        let customLabel = userConfig.profileLabels[configKey]

        self.title = customLabel ?? gcpConfig.displayName
        self.subtitle = [gcpConfig.account, gcpConfig.region].compactMap { $0 }.joined(separator: " · ")
        self.isActive = gcpConfig.isActive
        self.tag = userConfig.profileTags[configKey]
        self.isFavorite = userConfig.isFavorite(configKey)
        self.tokenStatus = nil
        self.timeRemaining = nil
        self.onTap = onTap
    }
}
