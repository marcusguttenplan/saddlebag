import Foundation

/// Result of a shell command execution
struct ShellResult: Sendable {
    let stdout: String
    let stderr: String
    let exitCode: Int32

    var succeeded: Bool { exitCode == 0 }
}

/// Thin wrapper around Process for running CLI commands
actor ShellService {
    /// Execute a shell command and return the result
    func run(_ command: String) async throws -> ShellResult {
        let process = Process()
        let stdoutPipe = Pipe()
        let stderrPipe = Pipe()

        process.executableURL = URL(fileURLWithPath: "/bin/zsh")
        process.arguments = ["-c", command]
        process.standardOutput = stdoutPipe
        process.standardError = stderrPipe

        // Inherit PATH so we can find aws, gcloud, etc.
        var environment = ProcessInfo.processInfo.environment
        // Ensure common paths are included
        let path = environment["PATH"] ?? ""
        let additionalPaths = [
            "/usr/local/bin",
            "/opt/homebrew/bin",
            "/usr/local/sbin",
            "\(NSHomeDirectory())/.local/bin"
        ]
        let combinedPath = (additionalPaths + [path]).joined(separator: ":")
        environment["PATH"] = combinedPath
        process.environment = environment

        try process.run()
        process.waitUntilExit()

        let stdoutData = stdoutPipe.fileHandleForReading.readDataToEndOfFile()
        let stderrData = stderrPipe.fileHandleForReading.readDataToEndOfFile()

        let stdout = String(data: stdoutData, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let stderr = String(data: stderrData, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""

        return ShellResult(
            stdout: stdout,
            stderr: stderr,
            exitCode: process.terminationStatus
        )
    }

    /// Execute a command and open a URL in the default browser
    func openURL(_ url: String) {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/open")
        process.arguments = [url]
        try? process.run()
    }
}
