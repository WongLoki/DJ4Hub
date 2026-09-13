import Foundation
import AVFoundation
import CoreAudio
import NativeAudio

struct AudioDevice: Identifiable {
    let id: String
    let name: String
    let input: Bool
    let output: Bool
    var module: Bool { name.range(of: "baiwang|quectel|qdc507|AC Interface|AS Interface", options: [.regularExpression, .caseInsensitive]) != nil }
}
@MainActor final class NativeVoice: ObservableObject {
    @Published var devices: [AudioDevice] = []
    @Published var mic = ""
    @Published var speaker = ""
    @Published var moduleInput = ""
    @Published var moduleOutput = ""
    @Published var status = "原生音频未连接"
    @Published var connected = false
    @Published var busy = false
    @Published var muted = false { didSet { dj_audio_gain(uplink, muted ? 0 : 1) } }
    @Published var volume: Double = 0.5 { didSet { dj_audio_gain(downlink, Float(volume)) } }
    private var uplink: UnsafeMutableRawPointer?
    private var downlink: UnsafeMutableRawPointer?
    private var token: String?
    private var lease: Task<Void, Never>?
    private var service: HubService?
    private var generation = 0
    var prepared: Bool { token != nil }

    func refreshDevices() {
        var address = AudioObjectPropertyAddress(mSelector: kAudioHardwarePropertyDevices, mScope: kAudioObjectPropertyScopeGlobal, mElement: kAudioObjectPropertyElementMain)
        var size: UInt32 = 0
        guard AudioObjectGetPropertyDataSize(AudioObjectID(kAudioObjectSystemObject), &address, 0, nil, &size) == noErr else { return }
        var ids = [AudioDeviceID](repeating: 0, count: Int(size) / MemoryLayout<AudioDeviceID>.size)
        let result = ids.withUnsafeMutableBytes { AudioObjectGetPropertyData(AudioObjectID(kAudioObjectSystemObject), &address, 0, nil, &size, $0.baseAddress!) }
        guard result == noErr else { return }
        func name(_ id: AudioDeviceID, _ selector: AudioObjectPropertySelector) -> String {
            var a = AudioObjectPropertyAddress(mSelector: selector, mScope: kAudioObjectPropertyScopeGlobal, mElement: kAudioObjectPropertyElementMain)
            var value: CFString?; var n = UInt32(MemoryLayout<CFString?>.size)
            let result = withUnsafeMutablePointer(to: &value) { AudioObjectGetPropertyData(id, &a, 0, nil, &n, UnsafeMutableRawPointer($0)) }
            guard result == noErr else { return "" }
            return value as String? ?? ""
        }
        func supports(_ id: AudioDeviceID, _ scope: AudioObjectPropertyScope) -> Bool {
            var a = AudioObjectPropertyAddress(mSelector: kAudioDevicePropertyStreams, mScope: scope, mElement: kAudioObjectPropertyElementMain)
            var n: UInt32 = 0
            return AudioObjectGetPropertyDataSize(id, &a, 0, nil, &n) == noErr && n > 0
        }
        devices = ids.map { AudioDevice(id: name($0, kAudioDevicePropertyDeviceUID), name: name($0, kAudioObjectPropertyName), input: supports($0, kAudioDevicePropertyScopeInput), output: supports($0, kAudioDevicePropertyScopeOutput)) }.filter { !$0.id.isEmpty }
        func select(_ old: String, _ candidates: [AudioDevice]) -> String { candidates.contains(where: { $0.id == old }) ? old : candidates.first?.id ?? "" }
        let before = [mic, speaker, moduleInput, moduleOutput]
        func preferred(_ selector: AudioObjectPropertySelector, input: Bool) -> [AudioDevice] {
            var a = AudioObjectPropertyAddress(mSelector: selector, mScope: kAudioObjectPropertyScopeGlobal, mElement: kAudioObjectPropertyElementMain)
            var id: AudioDeviceID = 0; var n = UInt32(MemoryLayout<AudioDeviceID>.size)
            _ = AudioObjectGetPropertyData(AudioObjectID(kAudioObjectSystemObject), &a, 0, nil, &n, &id)
            let uid = name(id, kAudioDevicePropertyDeviceUID)
            return devices.filter { (input ? $0.input : $0.output) && !$0.module }.sorted { ($0.id == uid ? 0 : 1) < ($1.id == uid ? 0 : 1) }
        }
        mic = select(mic, preferred(kAudioHardwarePropertyDefaultInputDevice, input: true))
        speaker = select(speaker, preferred(kAudioHardwarePropertyDefaultOutputDevice, input: false))
        moduleInput = select(moduleInput, devices.filter { $0.input && $0.module }); moduleOutput = select(moduleOutput, devices.filter { $0.output && $0.module })
        if connected && before != [mic, speaker, moduleInput, moduleOutput] { stopStreams(); status = "设备发生变化，已关闭音频，请重新连接" }
    }
    func stopStreams() {
        dj_audio_stop(uplink); uplink = nil
        dj_audio_stop(downlink); downlink = nil
        connected = false; muted = false
        status = token == nil ? "原生音频已断开" : "模块待机 · 电脑麦克风已关闭"
    }
    func prepareStandby(_ service: HubService) async throws {
        guard !connected else { return }
        guard !busy else { throw HubError(message: "音频正在初始化") }
        busy = true; defer { busy = false }
        try await ensureStandby(service)
        refreshDevices()
        status = "音频待机就绪 · 麦克风未开启 · 运营商通话尚未验证"
    }
    private func ensureStandby(_ service: HubService) async throws {
        let expectedGeneration = generation
        self.service = service
        if let token {
            do { _ = try await service.request("api/calls/audio/lease", method: "POST", token: token) }
            catch { self.token = nil; lease?.cancel() }
        }
        try Task.checkCancellation()
        guard generation == expectedGeneration else { throw CancellationError() }
        if token == nil {
            status = "正在初始化模块音频，USB 将短暂重新连接…"
            let result = try await service.request("api/calls/audio/prepare", method: "POST")
            guard let newToken = result.raw as? [String: Any], let value = newToken["token"] as? String, !value.isEmpty else { throw HubError(message: "服务未返回音频会话") }
            guard generation == expectedGeneration, !Task.isCancelled else {
                Task { _ = try? await service.request("api/calls/audio/stop", method: "POST", token: value) }
                throw CancellationError()
            }
            token = value
            lease?.cancel()
            lease = Task { [weak self] in
                while !Task.isCancelled {
                    do {
                        try await Task.sleep(nanoseconds: 10_000_000_000)
                        guard let self, let token = self.token else { return }
                        _ = try await service.request("api/calls/audio/lease", method: "POST", token: token)
                        self.refreshDevices()
                    } catch {
                        if !Task.isCancelled { self?.stopStreams(); self?.token = nil; self?.status = "模块连接丢失，音频已安全关闭" }
                        return
                    }
                }
            }
        }
        try Task.checkCancellation()
        guard generation == expectedGeneration else { throw CancellationError() }
    }
    func connect(_ service: HubService) async throws {
        guard !busy else { throw HubError(message: "音频正在初始化") }
        busy = true; defer { busy = false }
        stopStreams()
        try await ensureStandby(service)
        let permission = await AVCaptureDevice.requestAccess(for: .audio)
        guard permission else { throw HubError(message: "请在系统设置 → 隐私与安全性 → 麦克风中允许 DJ 4G Hub") }
        refreshDevices()
        guard !mic.isEmpty, !speaker.isEmpty, !moduleInput.isEmpty, !moduleOutput.isEmpty, mic != moduleInput, speaker != moduleOutput else { throw HubError(message: "未找到独立的电脑和模块音频设备") }
        var error: Int32 = 0
        uplink = dj_audio_start(mic, moduleOutput, &error)
        guard uplink != nil else { throw HubError(message: "原生麦克风连接失败：\(error)") }
        downlink = dj_audio_start(moduleInput, speaker, &error)
        guard downlink != nil else { stopStreams(); throw HubError(message: "原生扬声器连接失败：\(error)") }
        dj_audio_gain(downlink, Float(volume)); connected = true
        status = "原生双向音频已连接 · 请佩戴耳机，不保存录音"
    }
    func release() async {
        generation += 1
        stopStreams(); lease?.cancel(); lease = nil
        if let token, let service {
            do { _ = try await service.request("api/calls/audio/stop", method: "POST", token: token); status = "USB 已恢复" }
            catch { status = "恢复尚未确认；已停止续期，必要时重新插拔模块" }
        }
        token = nil
    }
}
