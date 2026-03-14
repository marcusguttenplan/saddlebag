import Foundation

/// An AWS SSO profile parsed from ~/.aws/config
struct AWSProfile: Identifiable, Hashable, Sendable {
    let id: String  // same as name
    let name: String
    let ssoSessionName: String
    let accountId: String
    let roleName: String
    let region: String
    let output: String

    /// Whether this profile has an admin role
    var isAdmin: Bool {
        roleName.localizedCaseInsensitiveContains("Administrator")
    }

    /// Whether this profile has a read-only role
    var isReadOnly: Bool {
        roleName.localizedCaseInsensitiveContains("ReadOnly")
    }

    /// Short role label for display
    var roleLabel: String {
        if isAdmin { return "Admin" }
        if isReadOnly { return "Read Only" }
        return roleName
    }

    init(
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
