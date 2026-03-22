import Foundation

/// An AWS SSO profile parsed from ~/.aws/config
public struct AWSProfile: Identifiable, Hashable, Sendable {
    public let id: String  // same as name
    public let name: String
    public let ssoSessionName: String
    public let accountId: String
    public let roleName: String
    public let region: String
    public let output: String

    /// Whether this profile has an admin role
    public var isAdmin: Bool {
        roleName.localizedCaseInsensitiveContains("Administrator")
    }

    /// Whether this profile has a read-only role
    public var isReadOnly: Bool {
        roleName.localizedCaseInsensitiveContains("ReadOnly")
    }

    /// Short role label for display
    public var roleLabel: String {
        if isAdmin { return "Admin" }
        if isReadOnly { return "Read Only" }
        return roleName
    }

    public init(
        name: String,
        ssoSessionName: String,
        accountId: String,
        roleName: String,
        region: String = "us-east-1",
        output: String = "json"
    ) {
        self.id = name
        self.name = name
        self.ssoSessionName = ssoSessionName
        self.accountId = accountId
        self.roleName = roleName
        self.region = region
        self.output = output
    }
}
