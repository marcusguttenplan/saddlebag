import Foundation

/// A secret from a cloud secret manager (GCP Secret Manager, AWS Secrets Manager)
public struct Secret: Identifiable, Hashable, Sendable {
    public var id: String { "\(provider.rawValue):\(project):\(name)" }

    /// Secret name (e.g. "api-database-url")
    public let name: String

    /// GCP project ID or AWS account/region
    public let project: String

    /// Cloud provider this secret belongs to
    public let provider: CloudProvider

    /// Labels/tags on the secret (key → value)
    public let labels: [String: String]

    /// When the secret was created
    public let createdAt: Date?

    // MARK: - Label Convention Accessors

    /// Organization label (e.g. "courseclear")
    public var org: String? { labels["org"] }

    /// Service label (e.g. "api", "web") — used as directory in env output
    public var service: String? { labels["service"] }

    /// Stage label (e.g. "dev", "prod") — used as file suffix .env.$stage
    public var stage: String? { labels["stage"] }

    /// Var label (e.g. "DATABASE_URL") — used as env var name
    public var varName: String? { labels["var"] }

    public init(
        name: String,
        project: String,
        provider: CloudProvider,
        labels: [String: String],
        createdAt: Date?
    ) {
        self.name = name
        self.project = project
        self.provider = provider
        self.labels = labels
        self.createdAt = createdAt
    }
}

/// Supported cloud providers for secrets
public enum CloudProvider: String, Sendable, CaseIterable, Codable {
    case gcp = "gcp"
    case aws = "aws"

    public var displayName: String {
        switch self {
        case .gcp: return "Google Cloud"
        case .aws: return "AWS"
        }
    }
}

// MARK: - GCP JSON Decoding

/// JSON structure from `gcloud secrets list --format=json`
public struct GCPSecretJSON: Decodable, Sendable {
    public let name: String               // "projects/PROJECT_NUM/secrets/SECRET_NAME"
    public let createTime: String?
    public let labels: [String: String]?

    /// Extract just the secret name from the full resource path
    public var secretName: String {
        // name is "projects/123456/secrets/my-secret"
        name.components(separatedBy: "/").last ?? name
    }

    public func toSecret(project: String) -> Secret {
        let date: Date? = createTime.flatMap { dateString in
            let formatter = ISO8601DateFormatter()
            formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            return formatter.date(from: dateString)
                ?? ISO8601DateFormatter().date(from: dateString)
        }

        return Secret(
            name: secretName,
            project: project,
            provider: .gcp,
            labels: labels ?? [:],
            createdAt: date
        )
    }
}
