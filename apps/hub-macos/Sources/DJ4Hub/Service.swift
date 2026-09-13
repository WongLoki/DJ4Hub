import Foundation
import AppKit

struct HubError: LocalizedError {
    let message: String
    var errorDescription: String? { message }
}
struct HubValue {
    let raw: Any
    init(_ raw: Any = [:]) { self.raw = raw }
    subscript(_ key: String) -> HubValue { HubValue((raw as? [String: Any])?[key] ?? NSNull()) }
    var text: String { if raw is NSNull { return "—" }; return (raw as? String) ?? (raw as? NSNumber)?.stringValue ?? "—" }
    var bool: Bool { (raw as? Bool) ?? false }
    var array: [HubValue] { (raw as? [Any] ?? []).map(HubValue.init) }
    var pretty: String { guard JSONSerialization.isValidJSONObject(raw), let data = try? JSONSerialization.data(withJSONObject: raw, options: [.prettyPrinted, .sortedKeys]), let text = String(data: data, encoding: .utf8) else { return text }; return text }
}

@MainActor final class HubService: ObservableObject {
    @Published var base = URL(string: "http://127.0.0.1:7575")!
    @Published var connected = false
    @Published var connectionText = "正在连接本地服务…"
    private var process: Process?
    private var log: FileHandle?
    private var starting = false
    var ownsService: Bool { process != nil }
    let session: URLSession
    init(session: URLSession? = nil) {
        if let session { self.session = session; return }
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = 140
        config.timeoutIntervalForResource = 150
        config.connectionProxyDictionary = [:]
        self.session = URLSession(configuration: config)
    }
    func request(_ path: String, method: String = "GET", body: [String: Any]? = nil, token: String? = nil) async throws -> HubValue {
        var request = URLRequest(url: try requestURL(path))
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("1", forHTTPHeaderField: "X-DJ4Hub-Audio")
        if path == "api/calls/audio/prepare" { request.setValue("1", forHTTPHeaderField: "X-DJ4Hub-Initialize") }
        if let token { request.setValue(token, forHTTPHeaderField: "X-DJ4Hub-Audio-Token") }
        if let body { request.httpBody = try JSONSerialization.data(withJSONObject: body) }
        let (data, response) = try await session.data(for: request)
        let value = HubValue((try? JSONSerialization.jsonObject(with: data)) ?? [:])
        guard let http = response as? HTTPURLResponse else { throw HubError(message: "本地服务响应无效") }
        guard (200..<300).contains(http.statusCode) else {
            let message = value["error"].raw as? String
            throw HubError(message: message?.isEmpty == false ? message! : "本地服务请求失败（HTTP \(http.statusCode)）")
        }
        return value
    }
    func requestURL(_ path: String) throws -> URL {
        guard let relative = URLComponents(string: path), relative.scheme == nil,
              relative.host == nil, relative.fragment == nil, !relative.path.contains(".."),
              var target = URLComponents(url: base.appendingPathComponent(relative.path), resolvingAgainstBaseURL: false) else {
            throw HubError(message: "无效的本地服务路径")
        }
        target.percentEncodedQuery = relative.percentEncodedQuery
        guard let url = target.url else { throw HubError(message: "无效的本地服务地址") }
        return url
    }
    private func probe(_ url: URL, demo: Bool) async -> Bool {
        var request = URLRequest(url: url.appendingPathComponent("api/health")); request.timeoutInterval = 1
        guard let (data, response) = try? await session.data(for: request), (response as? HTTPURLResponse)?.statusCode == 200,
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any], json["ok"] as? Bool == true,
              json["demo"] as? Bool == demo, json["esim_available"] is Bool else { return false }
        return true
    }
    func start() async {
        guard !starting, !connected else { return }
        starting = true
        defer { starting = false }
        let demo = ProcessInfo.processInfo.arguments.contains("--demo")
        let ports = demo ? [7578] : [7576, 7575]
        for port in ports {
            let url = URL(string: "http://127.0.0.1:\(port)")!
            if await probe(url, demo: demo) { base = url; connected = true; connectionText = "已连接现有服务 · \(port)"; return }
        }
        guard let resources = Bundle.main.resourceURL else { connectionText = "缺少 App 资源"; return }
        let binary = resources.appendingPathComponent("backend/bin/dj4ghub-macos")
        guard FileManager.default.isExecutableFile(atPath: binary.path) else { connectionText = "未打包设备服务；可先独立启动 Web 服务再重试"; return }
        let port = demo ? 7578 : 7575
        base = URL(string: "http://127.0.0.1:\(port)")!
        let child = Process(); child.executableURL = binary
        child.arguments = ["-listen", "127.0.0.1:\(port)"] + (demo ? ["-demo"] : [])
        let logDir = FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Library/Logs/DJ 4G Hub")
        do {
            try FileManager.default.createDirectory(at: logDir, withIntermediateDirectories: true)
            let logURL = logDir.appendingPathComponent("native-service.log")
            if !FileManager.default.fileExists(atPath: logURL.path) { FileManager.default.createFile(atPath: logURL.path, contents: nil) }
            log = try FileHandle(forWritingTo: logURL); try log?.seekToEnd()
            child.standardOutput = log; child.standardError = log
            try child.run(); process = child
            for _ in 0..<30 {
                if !child.isRunning { throw HubError(message: "设备服务启动失败，请查看 native-service.log；未停止其他服务") }
                if await probe(base, demo: demo) { connected = true; connectionText = "内置服务运行中 · \(port)"; return }
                try await Task.sleep(nanoseconds: 300_000_000)
            }
            throw HubError(message: "服务启动超时")
        } catch { connectionText = error.localizedDescription; stopOwned() }
    }
    func openWeb() { NSWorkspace.shared.open(base) }
    func stopOwned() {
        if let process, process.isRunning { process.terminate() }
        process = nil; try? log?.close(); log = nil
    }
}
