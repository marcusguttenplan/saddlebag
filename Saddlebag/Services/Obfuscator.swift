import Foundation

/// Types of sensitive data for context-aware redaction
enum SensitiveDataKind {
    case email
    case accountId
    case url
    case projectId
    case generic
}

/// Utility for redacting sensitive data in screenshot mode
enum Obfuscator {

    /// Dispatch to the appropriate redaction function
    static func redact(_ value: String, as kind: SensitiveDataKind) -> String {
        guard !value.isEmpty else { return value }
        switch kind {
        case .email: return redactEmail(value)
        case .accountId: return redactAccountId(value)
        case .url: return redactURL(value)
        case .projectId: return redactProjectId(value)
        case .generic: return redactGeneric(value)
        }
    }

    /// `user@company.com` → `u•••@c•••.com`
    static func redactEmail(_ email: String) -> String {
        let parts = email.split(separator: "@", maxSplits: 1)
        guard parts.count == 2 else { return redactGeneric(email) }

        let local = String(parts[0])
        let domain = String(parts[1])

        let redactedLocal = local.count > 1
            ? String(local.prefix(1)) + "•••"
            : "•••"

        let domainParts = domain.split(separator: ".", maxSplits: 1)
        let redactedDomain: String
        if domainParts.count == 2 {
            let domainName = String(domainParts[0])
            let tld = String(domainParts[1])
            redactedDomain = (domainName.count > 1
                ? String(domainName.prefix(1)) + "•••"
                : "•••") + "." + tld
        } else {
            redactedDomain = "•••"
        }

        return "\(redactedLocal)@\(redactedDomain)"
    }

    /// `123456789012` → `••••••••9012`
    static func redactAccountId(_ id: String) -> String {
        let digits = id.filter(\.isNumber)
        guard digits.count >= 4 else { return String(repeating: "•", count: max(id.count, 4)) }
        let suffix = String(digits.suffix(4))
        let masked = String(repeating: "•", count: digits.count - 4) + suffix
        return masked
    }

    /// `https://acme.awsapps.com/start` → `https://•••••.awsapps.com/start`
    static func redactURL(_ urlString: String) -> String {
        guard let url = URL(string: urlString),
              let host = url.host else {
            return redactGeneric(urlString)
        }

        let hostParts = host.split(separator: ".", maxSplits: 1)
        guard hostParts.count == 2 else { return redactGeneric(urlString) }

        let redactedSubdomain = String(repeating: "•", count: hostParts[0].count)
        let redactedHost = redactedSubdomain + "." + hostParts[1]

        return urlString.replacingOccurrences(of: host, with: redactedHost)
    }

    /// `my-project-prod-1234` → `••••-••••-1234`
    static func redactProjectId(_ projectId: String) -> String {
        let segments = projectId.split(separator: "-")
        guard segments.count >= 2 else { return redactGeneric(projectId) }

        // Keep last segment, redact the rest
        let redacted = segments.dropLast().map { String(repeating: "•", count: $0.count) }
        return (redacted + [String(segments.last!)]).joined(separator: "-")
    }

    /// `courseclear` → `c••••••••••r`
    static func redactGeneric(_ value: String) -> String {
        guard value.count > 2 else { return String(repeating: "•", count: value.count) }
        return String(value.prefix(1)) + String(repeating: "•", count: value.count - 2) + String(value.suffix(1))
    }
}
