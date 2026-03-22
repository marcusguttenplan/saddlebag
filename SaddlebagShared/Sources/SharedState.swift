import Foundation

/// Live shared state written to ~/.saddlebag/state.json
/// This is the contract between the app and the CLI.
public struct SharedState: Codable, Sendable {
    public var activeDesk: String?
    public var awsProfile: String?
    public var gcpConfig: String?
    public var gitEmail: String?
    public var lastUpdated: Date

    public init(
        activeDesk: String? = nil,
        awsProfile: String? = nil,
        gcpConfig: String? = nil,
        gitEmail: String? = nil
    ) {
        self.activeDesk = activeDesk
        self.awsProfile = awsProfile
        self.gcpConfig = gcpConfig
        self.gitEmail = gitEmail
        self.lastUpdated = Date()
    }

    // MARK: - Persistence

    private static var statePath: String {
        let dir = "\(NSHomeDirectory())/.saddlebag"
        return "\(dir)/state.json"
    }

    /// Read the current state from disk
    public static func read() -> SharedState {
        guard let data = FileManager.default.contents(atPath: statePath) else {
            return SharedState()
        }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        guard let state = try? decoder.decode(SharedState.self, from: data) else {
            return SharedState()
        }
        return state
    }

    /// Write state to disk
    public func write() throws {
        let dir = (Self.statePath as NSString).deletingLastPathComponent
        try FileManager.default.createDirectory(
            atPath: dir,
            withIntermediateDirectories: true
        )

        var copy = self
        copy.lastUpdated = Date()

        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        encoder.dateEncodingStrategy = .iso8601
        let data = try encoder.encode(copy)
        try data.write(to: URL(fileURLWithPath: Self.statePath))
    }
}
