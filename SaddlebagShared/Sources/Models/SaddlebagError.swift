import Foundation

public enum SaddlebagError: Error, LocalizedError {
    case shellExecutionFailed(command: String, exitCode: Int32, stderr: String)
    case missingExecutable(command: String)
    case commandFailed(reason: String)
    case unauthenticated(service: String)
    case invalidConfig(reason: String)
    case fileWriteError(path: String, reason: String)
    
    public var errorDescription: String? {
        switch self {
        case .shellExecutionFailed(let command, let exitCode, let stderr):
            return "Command '\(command)' failed with exit code \(exitCode): \(stderr)"
        case .missingExecutable(let command):
            return "Executable not found for command: \(command). Check your PATH."
        case .commandFailed(let reason):
            return reason
        case .unauthenticated(let service):
            return "Unauthenticated in \(service). Please log in."
        case .invalidConfig(let reason):
            return "Invalid configuration: \(reason)"
        case .fileWriteError(let path, let reason):
            return "Couldn't write to \(path): \(reason)"
        }
    }
}
