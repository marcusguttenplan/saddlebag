import Foundation

/// An SSO session definition from ~/.aws/config
public struct SSOSession: Identifiable, Hashable, Sendable {
    public let id: String  // same as name
    public let name: String
    public let startUrl: String
    public let region: String
    public let registrationScopes: String

    /// The portal domain extracted from the SSO start URL
    /// e.g. "https://courseclear.awsapps.com/start" → "courseclear"
    public var portalDomain: String {
        guard let url = URL(string: startUrl),
              let host = url.host else {
            return name
        }
        return host.components(separatedBy: ".").first ?? name
    }

    /// Human-readable portal name with capitalization
    public var portalDisplayName: String {
        portalDomain
            .replacingOccurrences(of: "-", with: " ")
            .split(separator: " ")
            .map { $0.prefix(1).uppercased() + $0.dropFirst() }
            .joined(separator: " ")
    }

    public init(
        name: String,
        startUrl: String,
        region: String = "us-east-1",
        registrationScopes: String = "sso:account:access"
    ) {
        self.id = name
        self.name = name
        self.startUrl = startUrl
        self.region = region
        self.registrationScopes = registrationScopes
    }
}
