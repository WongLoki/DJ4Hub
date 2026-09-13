import Foundation
import UserNotifications

@MainActor final class HubNotifications: NSObject, ObservableObject, UNUserNotificationCenterDelegate {
    @Published private(set) var latest = "暂无操作结果"
    @Published private(set) var permission = "首次操作时请求通知权限"
    private var lastAutomaticError = ""
    private var lastAutomaticDate = Date.distantPast
    private var incoming = IncomingAlertTracker()
    @Published var incomingSound = UserDefaults.standard.object(forKey: "incomingSound") as? Bool ?? true {
        didSet { UserDefaults.standard.set(incomingSound, forKey: "incomingSound") }
    }

    func receive(_ snapshot: HubValue) async {
        let events = incoming.update(sms: snapshot["sms"].array.map(\.text), ringing: snapshot["ringing"].array.map(\.text))
        for (enabled, title) in [(events.sms, "收到新短信"), (events.call, "有电话呼入")] where enabled {
            let content = UNMutableNotificationContent()
            content.title = "DJ 4G Hub · " + title
            content.body = "请打开客户端查看。"
            if incomingSound { content.sound = .default }
            do { try await UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil)) }
            catch { permission = "来电／短信通知发送失败，请检查系统通知权限" }
        }
    }

    func configure() { UNUserNotificationCenter.current().delegate = self }

    func authorize() async {
        let center = UNUserNotificationCenter.current()
        do {
            let settings = await center.notificationSettings()
            if settings.authorizationStatus == .notDetermined {
                _ = try await center.requestAuthorization(options: [.alert, .sound])
            }
            let updated = await center.notificationSettings()
            permission = updated.authorizationStatus == .authorized || updated.authorizationStatus == .provisional ? "系统通知已授权" : "系统通知未开启；可在 macOS 系统设置中允许"
        } catch { permission = "无法请求系统通知权限；操作结果仍可在这里查看" }
    }

    func report(_ detail: String, success: Bool, automatic: Bool = false) {
        latest = detail
        if automatic {
            guard detail != lastAutomaticError || Date().timeIntervalSince(lastAutomaticDate) >= 300 else { return }
            lastAutomaticError = detail; lastAutomaticDate = Date()
        }
        Task {
            if !automatic { await authorize() }
            let center = UNUserNotificationCenter.current()
            let settings = await center.notificationSettings()
            guard settings.authorizationStatus == .authorized || settings.authorizationStatus == .provisional else { return }
            let content = UNMutableNotificationContent()
            content.title = success ? "DJ 4G Hub · 操作完成" : "DJ 4G Hub · 操作未完成"
            content.body = success ? "操作已完成。" : "请打开客户端，在设置页查看详细结果。"
            let request = UNNotificationRequest(identifier: automatic ? "device-refresh-error" : UUID().uuidString, content: content, trigger: nil)
            do { try await center.add(request) }
            catch { permission = "系统通知发送失败；请在这里查看操作结果" }
        }
    }

    nonisolated func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification, withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .list, .sound])
    }
}
