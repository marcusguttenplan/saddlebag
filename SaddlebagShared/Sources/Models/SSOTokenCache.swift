import Foundation

/// Status of an SSO session token
public enum TokenExpiryStatus: Sendable {
    case valid
    case expiringSoon  // < 15 minutes remaining
    case expired
    case unknown
}

/// A cached SSO token from ~/.aws/sso/cache/*.json
public struct SSOTokenCache: Sendable {
    public let accessToken: String
    public let expiresAt: Date
    public let region: String
    public let startUrl: String

    public var isExpired: Bool {
        expiresAt <= Date()
    }

    /// Time remaining until expiry
    public var timeRemaining: TimeInterval {
        max(0, expiresAt.timeIntervalSinceNow)
    }

    /// Formatted time remaining string (e.g. "7h 42m" or "14m")
    public var timeRemainingFormatted: String {
        let remaining = timeRemaining
        if remaining <= 0 { return "Expired" }

        let hours = Int(remaining) / 3600
        let minutes = (Int(remaining) % 3600) / 60

        if hours > 0 {
            return "\(hours)h \(minutes)m"
        } else {
            return "\(minutes)m"
        }
    }

    public var expiryStatus: TokenExpiryStatus {
        if isExpired { return .expired }
        if timeRemaining < 900 { return .expiringSoon }  // 15 minutes
        return .valid
    }

    public init(accessToken: String, expiresAt: Date, region: String, startUrl: String) {
        self.accessToken = accessToken
        self.expiresAt = expiresAt
        self.region = region
        self.startUrl = startUrl
    }
}

/// JSON structure of AWS SSO cache files
public struct SSOTokenCacheJSON: Decodable, Sendable {
    public let accessToken: String?
    public let expiresAt: String?
    public let region: String?
    public let startUrl: String?

    public func toTokenCache() -> SSOTokenCache? {
        guard let accessToken, let expiresAtStr = expiresAt, let startUrl else {
            return nil
        }

        // AWS uses ISO 8601 format with UTC: "2025-03-14T20:00:00UTC"
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]

        // Try standard ISO 8601 first, then handle AWS's "UTC" suffix
        var date: Date?
        date = formatter.date(from: expiresAtStr)

        if date == nil {
            // AWS sometimes uses "2025-03-14T20:00:00UTC" format
            let cleaned = expiresAtStr
                .replacingOccurrences(of: "UTC", with: "Z")
                .replacingOccurrences(of: " ", with: "")
            date = formatter.date(from: cleaned)
        }

        if date == nil {
            // Fallback: try DateFormatter with common patterns
            let fallback = DateFormatter()
            fallback.dateFormat = "yyyy-MM-dd'T'HH:mm:ssZ"
            fallback.locale = Locale(identifier: "en_US_POSIX")
            date = fallback.date(from: expiresAtStr)
        }

        guard let expiresAt = date else { return nil }

        return SSOTokenCache(
            accessToken: accessToken,
            expiresAt: expiresAt,
            region: region ?? "us-east-1",
            startUrl: startUrl
        )
    }
}
