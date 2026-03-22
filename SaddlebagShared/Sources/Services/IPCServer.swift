import Foundation

/// Unix domain socket IPC server for CLI communication
/// Listens on /tmp/saddlebag.sock and dispatches JSON commands.
public final class IPCServer: @unchecked Sendable {
    private let socketPath = "/tmp/saddlebag.sock"
    private var fileDescriptor: Int32 = -1
    private var isRunning = false
    private let queue = DispatchQueue(label: "com.saddlebag.ipc", qos: .utility)

    /// Callback for handling incoming commands
    public var onCommand: ((IPCCommand) async -> IPCResponse)?

    public init() {}

    /// Start listening for connections
    public func start() {
        queue.async { [weak self] in
            self?.listen()
        }
    }

    /// Stop the server and clean up the socket file
    public func stop() {
        isRunning = false
        if fileDescriptor >= 0 {
            close(fileDescriptor)
            fileDescriptor = -1
        }
        unlink(socketPath)
    }

    private func listen() {
        // Clean up any stale socket
        unlink(socketPath)

        // Create socket
        fileDescriptor = socket(AF_UNIX, SOCK_STREAM, 0)
        guard fileDescriptor >= 0 else {
            print("[IPC] Failed to create socket: \(String(cString: strerror(errno)))")
            return
        }

        // Bind
        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let pathBytes = socketPath.utf8CString
        let maxLen = MemoryLayout.size(ofValue: addr.sun_path)
        precondition(pathBytes.count <= maxLen, "Socket path too long")
        withUnsafeMutableBytes(of: &addr.sun_path) { buf in
            for (i, byte) in pathBytes.prefix(maxLen).enumerated() {
                buf[i] = UInt8(bitPattern: byte)
            }
        }

        let bindResult = withUnsafePointer(to: &addr) { ptr in
            ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) { sockaddrPtr in
                bind(fileDescriptor, sockaddrPtr, socklen_t(MemoryLayout<sockaddr_un>.size))
            }
        }

        guard bindResult >= 0 else {
            print("[IPC] Failed to bind: \(String(cString: strerror(errno)))")
            close(fileDescriptor)
            return
        }

        // Listen
        guard Foundation.listen(fileDescriptor, 5) >= 0 else {
            print("[IPC] Failed to listen: \(String(cString: strerror(errno)))")
            close(fileDescriptor)
            return
        }

        isRunning = true
        print("[IPC] Listening on \(socketPath)")

        // Accept loop
        while isRunning {
            let clientFD = accept(fileDescriptor, nil, nil)
            guard clientFD >= 0 else {
                if isRunning {
                    print("[IPC] Accept error: \(String(cString: strerror(errno)))")
                }
                continue
            }

            // Handle each client on a separate task
            let handler = onCommand
            Task {
                await self.handleClient(fd: clientFD, handler: handler)
            }
        }
    }

    private func handleClient(fd: Int32, handler: ((IPCCommand) async -> IPCResponse)?) async {
        defer { close(fd) }

        // Read data from client
        var buffer = [UInt8](repeating: 0, count: 4096)
        let bytesRead = read(fd, &buffer, buffer.count)
        guard bytesRead > 0 else { return }

        let data = Data(buffer[0..<bytesRead])

        // Parse command
        guard let command = try? JSONDecoder().decode(IPCCommand.self, from: data) else {
            let errorResp = IPCResponse(status: "error", error: "invalid command JSON")
            sendResponse(errorResp, to: fd)
            return
        }

        // Dispatch to handler
        let response: IPCResponse
        if let handler {
            response = await handler(command)
        } else {
            response = IPCResponse(status: "error", error: "no handler registered")
        }

        sendResponse(response, to: fd)
    }

    private func sendResponse(_ response: IPCResponse, to fd: Int32) {
        guard var data = try? JSONEncoder().encode(response) else { return }
        data.append(contentsOf: [UInt8(ascii: "\n")])
        data.withUnsafeBytes { ptr in
            _ = write(fd, ptr.baseAddress!, ptr.count)
        }
    }

    deinit {
        stop()
    }
}

// MARK: - IPC Message Types

/// A JSON command received from the CLI
public struct IPCCommand: Codable, Sendable {
    public let action: String
    public var desk: String?

    public init(action: String, desk: String? = nil) {
        self.action = action
        self.desk = desk
    }
}

/// A JSON response sent back to the CLI
public struct IPCResponse: Codable, Sendable {
    public let status: String
    public var message: String?
    public var error: String?

    public init(status: String, message: String? = nil, error: String? = nil) {
        self.status = status
        self.message = message
        self.error = error
    }
}
