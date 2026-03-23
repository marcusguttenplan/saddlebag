import Foundation

/// Manages AWS SSO token lifecycle — checks cached tokens and triggers login
public actor SSOSessionService {
    private let shell: ShellService
    private let cachePath: String
    /// Active login process (so we can cancel/retry)
    private var loginProcess: Process?

    public init(shell: ShellService, cachePath: String? = nil) {
        self.shell = shell
        self.cachePath = cachePath ?? "\(NSHomeDirectory())/.aws/sso/cache"
    }

    /// Check all cached SSO tokens and match them to sessions by startUrl
    public func loadTokenStatuses(for sessions: [SSOSession]) -> [String: SSOTokenCache] {
        var statuses: [String: SSOTokenCache] = [:]

        let fileManager = FileManager.default
        guard let files = try? fileManager.contentsOfDirectory(atPath: cachePath) else {
            return statuses
        }

        // Load all cached tokens
        var allTokens: [SSOTokenCache] = []
        for file in files where file.hasSuffix(".json") {
            let filePath = (cachePath as NSString).appendingPathComponent(file)
            guard let data = fileManager.contents(atPath: filePath) else { continue }

            do {
                let cacheJSON = try JSONDecoder().decode(SSOTokenCacheJSON.self, from: data)
                if let token = cacheJSON.toTokenCache() {
                    allTokens.append(token)
                }
            } catch {
                continue
            }
        }

        // Match tokens to sessions by startUrl — pick the best (latest-expiring valid) token
        for session in sessions {
            let sessionUrl = normalizeUrl(session.startUrl)
            let matchingTokens = allTokens.filter { normalizeUrl($0.startUrl) == sessionUrl }

            let best = matchingTokens
                .sorted { a, b in
                    if a.isExpired != b.isExpired { return !a.isExpired }
                    return a.expiresAt > b.expiresAt
                }
                .first

            if let best {
                statuses[session.name] = best
            }
        }

        return statuses
    }

    /// Get the best token status for a specific profile
    public func tokenStatus(
        for profile: AWSProfile,
        sessions: [SSOSession],
        tokenStatuses: [String: SSOTokenCache]
    ) -> SSOTokenCache? {
        guard let session = sessions.first(where: { $0.name == profile.ssoSessionName }) else {
            return nil
        }
        return tokenStatuses[session.name]
    }

    /// Initiate SSO login for a profile.
    ///
    /// Uses the BROWSER env var trick: sets BROWSER to a script that writes
    /// the device auth URL to a temp file instead of opening a browser.
    /// This lets us reliably capture the URL without parsing stdout.
    ///
    /// Returns the device authorization URL, or nil if not captured.
    /// Calling this again cancels any previous in-flight login.
    public func login(profileName: String) async throws -> String? {
        // Kill any previous login process so this is retryable
        loginProcess?.terminate()
        loginProcess = nil

        // Temp file where the "browser" script will write the URL
        let urlFile = NSTemporaryDirectory() + "saddlebag_sso_url_\(ProcessInfo.processInfo.processIdentifier)"

        // Clean up any previous URL file
        try? FileManager.default.removeItem(atPath: urlFile)

        // Create a tiny script that captures the URL instead of opening a browser
        let browserScript = NSTemporaryDirectory() + "saddlebag_browser_\(ProcessInfo.processInfo.processIdentifier)"
        let scriptContent = "#!/bin/sh\necho \"$1\" > \"\(urlFile)\"\n"
        try scriptContent.write(toFile: browserScript, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: browserScript)

        let process = Process()
        let outputPipe = Pipe()

        process.executableURL = URL(fileURLWithPath: "/bin/zsh")
        process.arguments = ["-c", "aws sso login --profile \(profileName) 2>&1"]
        process.standardOutput = outputPipe
        process.standardError = outputPipe

        // Set up PATH and BROWSER
        var environment = ProcessInfo.processInfo.environment
        let path = environment["PATH"] ?? ""
        let additionalPaths = [
            "/usr/local/bin",
            "/opt/homebrew/bin",
            "/usr/local/sbin",
            "\(NSHomeDirectory())/.local/bin"
        ]
        environment["PATH"] = (additionalPaths + [path]).joined(separator: ":")
        // Redirect browser opening to our capture script
        environment["BROWSER"] = browserScript
        process.environment = environment

        loginProcess = process
        try process.run()

        // Wait briefly for the URL to be written by the browser script
        for _ in 0..<20 {
            try? await Task.sleep(for: .milliseconds(250))
            if FileManager.default.fileExists(atPath: urlFile) {
                if let url = try? String(contentsOfFile: urlFile, encoding: .utf8)
                    .trimmingCharacters(in: .whitespacesAndNewlines),
                   !url.isEmpty {
                    // Clean up
                    try? FileManager.default.removeItem(atPath: urlFile)
                    try? FileManager.default.removeItem(atPath: browserScript)

                    // Let the process continue in background waiting for auth
                    Task.detached { [weak process] in
                        process?.waitUntilExit()
                    }

                    return url
                }
            }
        }

        // Clean up if URL wasn't captured
        try? FileManager.default.removeItem(atPath: urlFile)
        try? FileManager.default.removeItem(atPath: browserScript)

        // Let process continue in background
        Task.detached { [weak process] in
            process?.waitUntilExit()
        }

        return nil
    }

    /// Check if a specific profile has a valid (non-expired) session
    public func isSessionValid(
        for profile: AWSProfile,
        sessions: [SSOSession],
        tokenStatuses: [String: SSOTokenCache]
    ) -> Bool {
        guard let token = tokenStatus(for: profile, sessions: sessions, tokenStatuses: tokenStatuses) else {
            return false
        }
        return !token.isExpired
    }

    // MARK: - Private

    private func normalizeUrl(_ urlString: String) -> String {
        guard var components = URLComponents(string: urlString) else {
            return urlString.lowercased()
        }
        components.fragment = nil
        components.query = nil
        var path = components.path
        while path.hasSuffix("/") {
            path = String(path.dropLast())
        }
        components.path = path
        return (components.url?.absoluteString ?? urlString).lowercased()
    }
}
