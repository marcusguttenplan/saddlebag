import SwiftUI
import ServiceManagement

@main
struct SaddlebagApp: App {
    @State private var viewModel = AccountsViewModel()

    init() {
        // Set the app icon programmatically from SF Symbol
        NSApplication.shared.applicationIconImage = SaddleIcon.appIcon()
    }

    var body: some Scene {
        // Menubar dropdown panel
        MenuBarExtra {
            MenuBarView(viewModel: viewModel)
                .frame(width: 380, height: 500)
        } label: {
            Image(nsImage: SaddleIcon.menuBarImage())
            if !viewModel.menuBarLabel.isEmpty {
                Text(viewModel.menuBarLabel)
            }
        }
        .menuBarExtraStyle(.window)
    }
}
