import Foundation
import SwiftUI

/// Kinds of sensitive data that can be redacted
public enum SensitiveDataKind: Sendable {
    case email
    case accountId
    case url
    case projectId
    case generic
}

/// Redacts sensitive values for screenshot mode
public struct Obfuscator: Sendable {
    /// Redact a value based on its data kind
    public static func redact(_ value: String, as kind: SensitiveDataKind) -> String {
        switch kind {
        case .email:
            return redactEmail(value)
        case .accountId:
            return redactAccountId(value)
        case .url:
            return redactSSOUrl(value)
        case .projectId:
            return redactProjectId(value)
        case .generic:
            return redactGeneric(value)
        }
    }

    // MARK: - Private Redaction Methods

    /// u•••@c•••.com
    private static func redactEmail(_ email: String) -> String {
        let parts = email.split(separator: "@", maxSplits: 1)
        guard parts.count == 2 else { return redactGeneric(email) }

        let user = parts[0]
        let domain = parts[1]
        let domainParts = domain.split(separator: ".", maxSplits: 1)

        let redactedUser = String(user.prefix(1)) + "•••"
        let redactedDomain: String
        if domainParts.count == 2 {
            redactedDomain = String(domainParts[0].prefix(1)) + "•••." + domainParts[1]
        } else {
            redactedDomain = "•••"
        }

        return "\(redactedUser)@\(redactedDomain)"
    }

    /// ••••••••9012
    private static func redactAccountId(_ id: String) -> String {
        let suffix = String(id.suffix(4))
        let dots = String(repeating: "•", count: max(0, id.count - 4))
        return dots + suffix
    }

    /// https://••••.awsapps.com/start
    private static func redactSSOUrl(_ url: String) -> String {
        guard let urlObj = URL(string: url),
              let host = urlObj.host else {
            return redactGeneric(url)
        }

        let hostParts = host.split(separator: ".")
        guard hostParts.count >= 2 else { return redactGeneric(url) }

        let redactedHost = "••••." + hostParts.dropFirst().joined(separator: ".")
        return url.replacingOccurrences(of: host, with: redactedHost)
    }

    /// ••••-••••-1234
    private static func redactProjectId(_ id: String) -> String {
        let parts = id.split(separator: "-")
        if parts.count >= 2 {
            let lastPart = parts.last!
            let redactedParts = parts.dropLast().map { _ in "••••" }
            return (redactedParts + [String(lastPart)]).joined(separator: "-")
        }
        return redactGeneric(id)
    }

    /// c••••••••••r
    private static func redactGeneric(_ value: String) -> String {
        guard value.count > 2 else { return "••" }
        let first = value.prefix(1)
        let last = value.suffix(1)
        let dots = String(repeating: "•", count: max(1, value.count - 2))
        return first + dots + last
    }
}
