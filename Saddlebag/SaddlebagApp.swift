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
private func handleIPCCommand(_ command: IPCCommand, viewModel: AccountsViewModel) -> IPCResponse {
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
        // TODO: Implement full desk switching via DeskService
        // For now, write the desk name to state
        var state = SharedState.read()
        state.activeDesk = deskName
        do {
            try state.write()
        } catch {
            return IPCResponse(status: "error", error: error.localizedDescription)
        }
        return IPCResponse(status: "ok", message: "switched to \(deskName)")

    case "refresh":
        Task { @MainActor in
            await viewModel.refresh()
        }
        return IPCResponse(status: "ok", message: "refreshing")

    default:
        return IPCResponse(status: "error", error: "unknown action: \(command.action)")
    }
}
