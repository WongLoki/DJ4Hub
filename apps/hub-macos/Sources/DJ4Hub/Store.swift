import Foundation
import SwiftUI

enum HubPage: String, CaseIterable, Identifiable {
    case overview = "概览", sms = "短信", phone = "电话", esim = "eSIM / 卡片", network = "网络", at = "AT 调试", settings = "设置"
    var id: String { rawValue }
    var symbol: String {
        switch self { case .overview: return "square.grid.2x2"; case .sms: return "message"; case .phone: return "phone"; case .esim: return "simcard"; case .network: return "network"; case .at: return "terminal"; case .settings: return "gearshape" }
    }
}
@MainActor final class HubStore: ObservableObject {
    let service = HubService()
    let voice = NativeVoice()
    let notifications = HubNotifications()
    @Published var page: HubPage = .overview
    @Published var status = HubValue()
    @Published var traffic = HubValue()
    @Published var trafficHistory = TrafficHistory()
    @Published var messages: [HubValue] = []
    @Published var calls: [HubValue] = []
    @Published var network = HubValue()
    @Published var activity = HubValue()
    @Published var esim = HubValue()
    @Published var busy = false
    @Published var diagnostic = "尚未检测"
    @Published var console = "等待命令"
    @Published var ready = false
    @Published private(set) var polling = false
    @Published private(set) var deviceConnected = false
    @Published private(set) var healthKnown = false
    private var alertsTask: Task<Void, Never>?
    private var standbyTask: Task<Void, Never>?
    private var standbyAttempted = false
    private var shuttingDown = false
    @Published var backgroundAudio = UserDefaults.standard.object(forKey: "backgroundAudio") as? Bool ?? true {
        didSet {
            UserDefaults.standard.set(backgroundAudio, forKey: "backgroundAudio")
            if backgroundAudio { standbyAttempted = false; scheduleStandby() }
            else { stopBackgroundPreparation(); Task { await voice.release() } }
        }
    }
    func stopBackgroundPreparation(shutdown: Bool = false) { shuttingDown = shuttingDown || shutdown; standbyTask?.cancel(); standbyTask = nil }
    private func scheduleStandby() {
        guard !shuttingDown, backgroundAudio, ready, deviceConnected, !busy, !voice.busy,
              !voice.prepared, !voice.connected, !standbyAttempted, standbyTask == nil,
              !ProcessInfo.processInfo.arguments.contains("--demo") else { return }
        standbyTask = Task { [weak self] in
            guard let self else { return }
            defer { self.standbyTask = nil }
            do {
                try await Task.sleep(nanoseconds: 2_000_000_000)
                guard self.backgroundAudio, self.deviceConnected, !self.busy, !self.voice.busy else { return }
                let calls = try await self.service.request("api/calls")["calls"].array
                guard calls.isEmpty else { return }
                try Task.checkCancellation()
                self.standbyAttempted = true
                try await self.voice.prepareStandby(self.service)
            } catch is CancellationError { }
            catch {
                self.standbyAttempted = true
                self.voice.status = "后台音频准备未完成：\(error.localizedDescription)。可在电话页手动重试。"
                self.notifications.report(self.voice.status, success: false, automatic: true)
            }
        }
    }
    func start() async {
        notifications.configure()
        await notifications.authorize()
        await service.start(); ready = service.connected
        if ready { await refresh(); voice.refreshDevices() }
        else { notifications.report(service.connectionText, success: false, automatic: true) }
        if ready && alertsTask == nil {
            alertsTask = Task { [weak self] in
                while !Task.isCancelled {
                    if let self, let snapshot = try? await self.service.request("api/alerts") { await self.notifications.receive(snapshot) }
                    do { try await Task.sleep(nanoseconds: 3_000_000_000) } catch { return }
                }
            }
        }
    }
    func refresh() async {
        guard ready, !polling, !busy, !voice.busy else { return }
        polling = true; defer { polling = false }
        do {
            let health = try await service.request("api/health")
            healthKnown = true
            deviceConnected = health["ok"].bool && !health["port"].text.isEmpty && health["port"].text != "—" && health["discovery_error"].text.isEmpty
            if !deviceConnected { standbyAttempted = false }
            scheduleStandby()
            // Poll only the visible page, keeping USB work demand-driven.
            switch page {
            case .overview:
                status = try await service.request("api/status")
                traffic = try await service.request("api/network/traffic")
                trafficHistory.record(traffic)
                activity = try await service.request("api/network/activity")
            case .sms: messages = try await service.request("api/sms").array
            case .phone:
                let next = try await service.request("api/calls")["calls"].array
                if !calls.isEmpty && next.isEmpty { voice.stopStreams() }
                calls = next
            case .network: network = try await service.request("api/network")
            case .esim: esim = try await service.request("api/esim")
            case .settings, .at: break
            }
            // Continue monitoring hangup when a different native page is visible.
            if page != .phone && (!calls.isEmpty || voice.connected) {
                let next = try await service.request("api/calls")["calls"].array
                if !calls.isEmpty && next.isEmpty { voice.stopStreams() }
                calls = next
            }
        } catch { healthKnown = false; deviceConnected = false; notifications.report(error.localizedDescription, success: false, automatic: true) }
    }
    func run(_ operation: @escaping () async throws -> Void) {
        guard !busy else { return }; busy = true
        Task {
            do { try await operation(); notifications.report("操作完成", success: true) }
            catch { notifications.report(error.localizedDescription, success: false) }
            busy = false; await refresh()
        }
    }
    func action(_ path: String, method: String = "POST", body: [String: Any] = [:]) {
        run { _ = try await self.service.request(path, method: method, body: body) }
    }
    func phone(_ action: String, number: String = "", audio: Bool) {
        run {
            if audio && (action == "dial" || action == "answer") {
                if let pending = self.standbyTask { await pending.value }
                try await self.voice.connect(self.service)
            }
            do { _ = try await self.service.request("api/calls", method: "POST", body: ["action": action, "number": number]) }
            catch { self.voice.stopStreams(); throw error }
            if action == "hangup" { self.voice.stopStreams() }
        }
    }
}
