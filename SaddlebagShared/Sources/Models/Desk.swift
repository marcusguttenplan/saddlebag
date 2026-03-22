import Foundation

/// A desk definition — a developer context bundle
/// Loaded from ~/.saddlebag/desks/*.toml
public struct Desk: Identifiable, Codable, Sendable {
    public let id: String        // filename without .toml
    public var name: String
    public var awsProfile: String?
    public var gcpConfig: String?
    public var gitEmail: String?
    public var gitName: String?
    public var sshKey: String?
    public var workingDir: String?
    public var envVars: [String: String]

    public init(
        id: String,
        name: String,
        awsProfile: String? = nil,
        gcpConfig: String? = nil,
        gitEmail: String? = nil,
        gitName: String? = nil,
        sshKey: String? = nil,
        workingDir: String? = nil,
        envVars: [String: String] = [:]
    ) {
        self.id = id
        self.name = name
        self.awsProfile = awsProfile
        self.gcpConfig = gcpConfig
        self.gitEmail = gitEmail
        self.gitName = gitName
        self.sshKey = sshKey
        self.workingDir = workingDir
        self.envVars = envVars
    }
}
