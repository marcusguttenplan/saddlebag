import SwiftUI
import ServiceManagement
import SaddlebagShared

@main
struct SaddlebagApp: App {
    @State private var viewModel = AccountsViewModel()
    private let ipcServer = IPCServer()

    init() {
        // Set the app icon programmatically from SF Symbol
        NSApplication.shared.applicationIconImage = SaddleIcon.appIcon()
    }

    var body: some Scene {
        // Menubar dropdown panel
        MenuBarExtra {
            MenuBarView(viewModel: viewModel)
                .frame(width: 380, height: 500)
                .onAppear {
                    startIPCServer()
                }
        } label: {
            Image(nsImage: SaddleIcon.menuBarImage())
            if !viewModel.menuBarLabel.isEmpty {
                Text(viewModel.menuBarLabel)
                    .font(.system(size: 11, weight: .medium))
                    .baselineOffset(-0.5)
                    .padding(.leading, 2)
            }
        }
        .menuBarExtraStyle(.window)
    }

    private func startIPCServer() {
        ipcServer.onCommand = { [viewModel] command in
            await handleIPCCommand(command, viewModel: viewModel)
        }
        ipcServer.start()
    }
}

/// Handle IPC commands from the CLI
@MainActor
private func handleIPCCommand(_ command: IPCCommand, viewModel: AccountsViewModel) async -> IPCResponse {
    switch command.action {
    case "status":
        let state = SharedState.read()
        return IPCResponse(
            status: "ok",
            message: "desk=\(state.activeDesk ?? "none") aws=\(state.awsProfile ?? "none") gcp=\(state.gcpConfig ?? "none")"
        )

    case "switch":
        guard let deskName = command.desk else {
            return IPCResponse(status: "error", error: "missing desk name")
        }
        let success = await viewModel.switchDeskByID(deskName)
        if success {
            return IPCResponse(status: "ok", message: "switched to \(deskName)")
        } else {
            return IPCResponse(status: "error", error: "desk '\(deskName)' not found in ~/.saddlebag/desks/")
        }

    case "refresh":
        Task { @MainActor in
            await viewModel.refresh()
        }
        return IPCResponse(status: "ok", message: "refreshing")

    default:
        return IPCResponse(status: "error", error: "unknown action: \(command.action)")
    }
}
