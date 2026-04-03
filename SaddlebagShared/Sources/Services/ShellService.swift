import Foundation

/// Result of a shell command execution
public struct ShellResult: Sendable {
    public let stdout: String
    public let stderr: String
    public let exitCode: Int32

    public var succeeded: Bool { exitCode == 0 }

    public init(stdout: String, stderr: String, exitCode: Int32) {
        self.stdout = stdout
        self.stderr = stderr
        self.exitCode = exitCode
    }
}

/// Thin wrapper around Process for running CLI commands
public actor ShellService {
    public init() {}

    /// Execute a shell command and return the result
    public func run(_ command: String) async throws -> ShellResult {
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

        let stdoutTask = Task.detached {
            try? stdoutPipe.fileHandleForReading.readToEnd()
        }
        let stderrTask = Task.detached {
            try? stderrPipe.fileHandleForReading.readToEnd()
        }

        let stdoutData = (await stdoutTask.value) ?? Data()
        let stderrData = (await stderrTask.value) ?? Data()

        process.waitUntilExit()

        let stdout = String(data: stdoutData, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let stderr = String(data: stderrData, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""

        let result = ShellResult(
            stdout: stdout,
            stderr: stderr,
            exitCode: process.terminationStatus
        )

        guard result.succeeded else {
            throw SaddlebagError.shellExecutionFailed(
                command: command,
                exitCode: result.exitCode,
                stderr: result.stderr
            )
        }

        return result
    }

    /// Execute a command and open a URL in the default browser
    public func openURL(_ url: String) {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/open")
        process.arguments = [url]
        try? process.run()
    }
}
