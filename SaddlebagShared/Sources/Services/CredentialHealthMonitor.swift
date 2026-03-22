import Foundation
import UserNotifications

/// Monitors credential health and sends macOS notifications
/// when tokens transition from valid → expiringSoon → expired.
public final class CredentialHealthMonitor: @unchecked Sendable {
    private var previousStatuses: [String: TokenExpiryStatus] = [:]
    private let notificationCenter = UNUserNotificationCenter.current()

    public init() {}

    /// Request notification permission on first use
    public func requestPermission() {
        notificationCenter.requestAuthorization(options: [.alert, .sound, .badge]) { granted, error in
            if let error {
                print("[HealthMonitor] Notification permission error: \(error)")
            }
            print("[HealthMonitor] Notifications \(granted ? "granted" : "denied")")
        }
    }

    /// Check token statuses and send notifications for state transitions
    public func evaluate(sessions: [String: SSOTokenCache]) {
        for (sessionName, token) in sessions {
            let currentStatus = token.expiryStatus
            let previousStatus = previousStatuses[sessionName]

            // Only notify on transitions
            if let previous = previousStatus, previous != currentStatus {
                switch currentStatus {
                case .expiringSoon:
                    sendNotification(
                        title: "⚠️ SSO Token Expiring Soon",
                        body: "\(sessionName) expires in \(token.timeRemainingFormatted). Re-authenticate to stay connected.",
                        identifier: "expiring-\(sessionName)"
                    )

                case .expired:
                    sendNotification(
                        title: "🔴 SSO Token Expired",
                        body: "\(sessionName) session has expired. Open Saddlebag to log in again.",
                        identifier: "expired-\(sessionName)"
                    )

                case .valid:
                    // Token was renewed — clear any pending notifications
                    notificationCenter.removeDeliveredNotifications(
                        withIdentifiers: ["expiring-\(sessionName)", "expired-\(sessionName)"]
                    )

                case .unknown:
                    break
                }
            }

            previousStatuses[sessionName] = currentStatus
        }
    }

    /// Reset tracked state (e.g. on app relaunch)
    public func reset() {
        previousStatuses.removeAll()
    }

    // MARK: - Private

    private func sendNotification(title: String, body: String, identifier: String) {
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        content.sound = .default
        content.categoryIdentifier = "SADDLEBAG_CREDENTIAL"

        let request = UNNotificationRequest(
            identifier: identifier,
            content: content,
            trigger: nil  // deliver immediately
        )

        notificationCenter.add(request) { error in
            if let error {
                print("[HealthMonitor] Failed to send notification: \(error)")
            }
        }
    }
}

// MARK: - TokenExpiryStatus Equatable conformance

extension TokenExpiryStatus: Equatable {}
