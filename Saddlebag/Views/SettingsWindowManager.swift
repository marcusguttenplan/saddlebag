import SwiftUI
import AppKit

/// Manages the main app window lifecycle (Tailscale-style)
@MainActor
final class AppWindowManager: NSObject, NSWindowDelegate {
    static let shared = AppWindowManager()

    private var window: NSWindow?

    func open(viewModel: AccountsViewModel) {
        // If window exists, bring it to front
        if let window {
            window.makeKeyAndOrderFront(nil)
            NSApp.activate(ignoringOtherApps: true)
            return
        }

        // Create the main app view
        let mainView = MainAppView(viewModel: viewModel)
        let hostingView = NSHostingView(rootView: mainView)

        // Create window
        let newWindow = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 800, height: 540),
            styleMask: [.titled, .closable, .resizable, .miniaturizable],
            backing: .buffered,
            defer: false
        )
        newWindow.title = "Saddlebag"
        newWindow.contentView = hostingView
        newWindow.center()
        newWindow.isReleasedWhenClosed = false
        newWindow.delegate = self
        newWindow.titlebarAppearsTransparent = true
        newWindow.toolbarStyle = .unified

        // Activate app and show window
        NSApp.setActivationPolicy(.accessory)
        newWindow.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)

        self.window = newWindow
    }

    /// Bring window to front if visible
    func bringAllToFront() {
        if let window, window.isVisible {
            window.makeKeyAndOrderFront(nil)
            NSApp.activate(ignoringOtherApps: true)
        }
    }

    // MARK: - NSWindowDelegate

    func windowDidBecomeMain(_ notification: Notification) {
        NSApp.activate(ignoringOtherApps: true)
    }

    func windowWillClose(_ notification: Notification) {
        window = nil
    }
}
