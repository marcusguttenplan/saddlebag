import Foundation

/// Manages AWS SSO token lifecycle — checks cached tokens and triggers login
actor SSOSessionService {
    private let shell: ShellService
    private let cachePath: String

    init(shell: ShellService, cachePath: String? = nil) {
        self.shell = shell
        self.cachePath = cachePath ?? "\(NSHomeDirectory())/.aws/sso/cache"
    }

    /// Check all cached SSO tokens and match them to sessions by startUrl
    func loadTokenStatuses(for sessions: [SSOSession]) -> [String: SSOTokenCache] {
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
                // Skip invalid cache files (some are OIDC registrations, not tokens)
                continue
            }
        }

        // Match tokens to sessions by startUrl — pick the best (latest-expiring valid) token
        for session in sessions {
            let sessionUrl = normalizeUrl(session.startUrl)

            let matchingTokens = allTokens.filter { normalizeUrl($0.startUrl) == sessionUrl }

            // Prefer non-expired tokens; among those, pick the one expiring latest
            let best = matchingTokens
                .sorted { a, b in
                    // Non-expired tokens first, then by latest expiry
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
    func tokenStatus(
        for profile: AWSProfile,
        sessions: [SSOSession],
        tokenStatuses: [String: SSOTokenCache]
    ) -> SSOTokenCache? {
        // Find the session for this profile
        guard let session = sessions.first(where: { $0.name == profile.ssoSessionName }) else {
            return nil
        }

        // Check if we have a token for this session
        return tokenStatuses[session.name]
    }

    /// Initiate SSO login for a profile
    func login(profileName: String) async -> Bool {
        do {
            let result = try await shell.run("aws sso login --profile \(profileName)")
            return result.succeeded
        } catch {
            print("SSO login failed for \(profileName): \(error)")
            return false
        }
    }

    /// Check if a specific profile has a valid (non-expired) session
    func isSessionValid(
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

    /// Normalize a URL for comparison (strip trailing slashes, fragments, query params)
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
