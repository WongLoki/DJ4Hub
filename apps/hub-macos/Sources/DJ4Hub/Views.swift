import SwiftUI
import AppKit

@MainActor func confirm(_ title: String, _ detail: String, action: () -> Void) {
    let alert = NSAlert(); alert.messageText = title; alert.informativeText = detail
    alert.alertStyle = .warning; alert.addButton(withTitle: "继续"); alert.addButton(withTitle: "取消")
    if alert.runModal() == .alertFirstButtonReturn { action() }
}
struct Panel<Content: View>: View {
    @Environment(\.colorScheme) private var scheme
    let title: String
    var height: CGFloat? = nil
    @ViewBuilder var content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            if !title.isEmpty { Text(title).font(.system(size: 12, weight: .semibold)).foregroundStyle(.secondary) }
            content
        }.padding(20).frame(maxWidth: .infinity, alignment: .leading).frame(height: height)
            .background(Color(nsColor: .controlBackgroundColor).opacity(scheme == .dark ? 0.85 : 0.94), in: RoundedRectangle(cornerRadius: 19))
            .shadow(color: .black.opacity(scheme == .dark ? 0.14 : 0.035), radius: 9, x: 0, y: 3)
    }
}
struct RawDetails: View {
    let title: String
    let value: HubValue
    var body: some View { DisclosureGroup(title) { ScrollView { RecordDetails(value: value).padding(.top, 10) }.frame(maxHeight: 280) }.font(.system(size: 12, weight: .medium)) }
}
struct HubRoot: View {
    @ObservedObject var store: HubStore
    @ObservedObject var service: HubService
    let timer = Timer.publish(every: 5, on: .main, in: .common).autoconnect()
    var body: some View {
        HStack(spacing: 0) {
            VStack(alignment: .leading, spacing: 24) {
                HStack(spacing: 10) {
                    if let image = NSImage(named: NSImage.Name("AppIcon")) { Image(nsImage: image).resizable().frame(width: 30, height: 30) }
                    VStack(alignment: .leading, spacing: 3) { Text("DJ 4G Hub").font(.system(size: 13, weight: .semibold)); Text("设备工作空间").font(.system(size: 10)).foregroundStyle(.secondary) }
                }.padding(.horizontal, 12).padding(.top, 14)
                VStack(alignment: .leading, spacing: 5) {
                    navigation(.overview)
                    section("通信")
                    navigation(.sms); navigation(.phone)
                    section("设备")
                    navigation(.esim); navigation(.network); navigation(.at)
                }
                Spacer(minLength: 20)
                VStack(alignment: .leading, spacing: 8) {
                    navigation(.settings)
                    Button { service.openWeb() } label: { HStack { Image(systemName: "arrow.up.right.square"); Text("打开 Web"); Spacer(); Image(systemName: "arrow.up.right").font(.system(size: 9)) }.padding(10).contentShape(Rectangle()) }.buttonStyle(.plain).foregroundStyle(.secondary)
                    Text("LOCAL-FIRST · USB").font(.system(size: 9, weight: .medium)).tracking(1).foregroundStyle(.tertiary).padding(.horizontal, 10)
                }.font(.system(size: 12)).padding(.bottom, 16)
            }.padding(.horizontal, 10).frame(width: 174)
                .background { Rectangle().fill(.ultraThinMaterial).ignoresSafeArea(edges: .top) }
            VStack(alignment: .leading, spacing: 0) {
                    HStack(alignment: .center) {
                        VStack(alignment: .leading, spacing: 3) { Text(store.page.rawValue).font(.system(size: 21, weight: .semibold)); Text(subtitle).font(.system(size: 11)).foregroundStyle(.secondary) }
                        Spacer()
                        HStack(spacing: 6) {
                            Circle().fill(store.deviceConnected ? HubStyle.accent : Color.secondary).frame(width: 6, height: 6)
                            Text(ProcessInfo.processInfo.arguments.contains("--demo") ? "演示预览" : (!store.ready ? "服务未连接" : (!store.healthKnown ? "正在确认设备" : (store.deviceConnected ? "设备已连接" : "未检测到设备")))).font(.system(size: 11, weight: .medium)).foregroundStyle(.secondary)
                        }.help("\(service.connectionText)。设备连接不代表互联网可用，请在网络页检测。")
                        if store.busy || store.voice.busy { ProgressView().controlSize(.small) }
                        Button { Task { await store.refresh() } } label: {
                            Group { if store.polling { ProgressView().controlSize(.small) } else { Image(systemName: "arrow.clockwise") } }.frame(width: 16, height: 16)
                        }.buttonStyle(ToolbarButtonStyle()).disabled(store.polling || store.busy || !store.ready).help("刷新当前页面").accessibilityLabel("刷新当前页面")
                    }.padding(.horizontal, 24).padding(.vertical, 12)
                Divider().opacity(0.5)
                ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    if !store.ready { Panel(title: "连接设备服务") { Text(service.connectionText); Button("重试连接") { Task { await store.start() } }; Text("Web 控制台保持独立可用，客户端不会停止已有服务。").foregroundStyle(.secondary) } }
                    Group {
                        switch store.page {
                        case .overview: OverviewPage(store: store)
                        case .sms: SMSPage(store: store)
                        case .phone: PhonePage(store: store, voice: store.voice)
                        case .esim: ESIMPage(store: store)
                        case .network: NetworkPage(store: store)
                        case .at: ATPage(store: store)
                        case .settings: SettingsPage(service: service, notifications: store.notifications)
                        }
                    }.disabled(!store.ready || store.busy)
                }.padding(24).frame(maxWidth: store.page == .settings ? 820 : 1120, alignment: .leading).frame(maxWidth: .infinity, alignment: .leading)
            }.id(store.page)
            }
        }.background(HubBackdrop()).tint(HubStyle.accent).buttonStyle(HubButtonStyle()).disclosureGroupStyle(HubDisclosureStyle()).controlSize(.large).frame(minWidth: 980, minHeight: 680)
            .task { await store.start() }
            .onChange(of: store.page) { _ in Task { await store.refresh() } }
            .onReceive(timer) { _ in Task { await store.refresh() } }
    }
    private var subtitle: String {
        switch store.page {
        case .overview: return "设备、网络与连接活动，一目了然"
        case .sms: return "收发短信与验证码"
        case .phone: return "SIM 语音通话 · 原生电脑音频"
        case .esim: return "卡片资料与 Profile 管理"
        case .network: return "蜂窝数据、USB 网卡与出口诊断"
        case .at: return "直接与设备通信"
        case .settings: return "客户端、服务与 Web 备用入口"
        }
    }
    private func section(_ title: String) -> some View { Text(title).font(.system(size: 10, weight: .medium)).foregroundStyle(.tertiary).padding(.horizontal, 11).padding(.top, 18).padding(.bottom, 4) }
    private func navigation(_ page: HubPage) -> some View {
        Button { store.page = page } label: {
            HStack(spacing: 10) { Image(systemName: page.symbol).font(.system(size: 14)).frame(width: 18); Text(page.rawValue).font(.system(size: 13, weight: store.page == page ? .semibold : .regular)); Spacer() }
                .padding(.horizontal, 11).frame(minHeight: 44).contentShape(Rectangle())
                .background(Color.primary.opacity(store.page == page ? 0.075 : 0), in: RoundedRectangle(cornerRadius: 8))
        }.buttonStyle(.plain).accessibilityAddTraits(store.page == page ? .isSelected : [])
    }
}
struct OverviewPage: View {
    @ObservedObject var store: HubStore
    var body: some View {
        HStack(spacing: 20) {
            let carrier = CarrierDisplay(raw: store.status["operator"].text)
            VStack(alignment: .leading, spacing: 5) {
                InfoPair(title: "运营商", value: carrier.name)
                if let code = carrier.code { Text("网络代码 · \(code)").font(.system(size: 10)).foregroundStyle(.secondary) }
            }.frame(maxWidth: .infinity, alignment: .leading).help("当前接入的蜂窝运营商；漫游时可能与 SIM 卡所属运营商不同。")
            InfoPair(title: "网络制式", value: store.status["network_mode"].text)
            InfoPair(title: "SIM 卡", value: store.status["sim_inserted"].bool ? "已插入" : "未检测到")
            InfoPair(title: "物理出口", value: store.activity["physical_interface"].text)
        }.padding(.vertical, 6)
        HStack(alignment: .top, spacing: 14) {
            Panel(title: "蜂窝信号", height: 168) {
                HStack(alignment: .firstTextBaseline, spacing: 5) { Text(store.status["signal_dbm"].text).font(.system(size: 35, weight: .semibold, design: .rounded)); Text("dBm").font(.caption).foregroundStyle(.secondary) }
                Spacer(minLength: 14)
                HStack { InfoPair(title: "注册状态", value: store.status["reg_status_text"].text); Image(systemName: "antenna.radiowaves.left.and.right").font(.system(size: 30, weight: .light)).foregroundStyle(HubStyle.accent) }
            }.frame(maxWidth: .infinity)
            SpeedPanel(title: "实时下载", values: store.trafficHistory.download, color: HubStyle.download)
            SpeedPanel(title: "实时上传", values: store.trafficHistory.upload, color: HubStyle.upload)
        }
        HStack(alignment: .top, spacing: 14) {
            Panel(title: "应用连接", height: 156) {
                HStack { Text("\(store.activity["connections"].array.count)").font(.system(size: 35, weight: .semibold, design: .rounded)); Spacer(); StatusPill(text: store.activity["physical_active"].bool ? "接口活跃" : "接口未活跃", active: store.activity["physical_active"].bool) }
                HStack { InfoPair(title: "应用", value: "\(Set(store.activity["connections"].array.map { $0["process"].text }).count)"); InfoPair(title: "链路", value: store.activity["tunnel_interface"].text) }
            }
            Panel(title: "本次会话流量", height: 156) {
                Text(store.traffic["available"].bool ? bytes(store.traffic["session_total_bytes"]) : "—").font(.system(size: 30, weight: .semibold, design: .rounded))
                HStack(spacing: 25) { traffic("下载", "session_rx_bytes"); traffic("上传", "session_tx_bytes") }
            }
        }
        Panel(title: "联网活动") {
            HStack { Text(store.activity["physical_interface"].text); Image(systemName: "arrow.right"); Text(store.activity["tunnel_interface"].text); Spacer(); Text("\(store.activity["connections"].array.count) 个连接").foregroundStyle(.secondary) }.font(.callout)
            if store.activity["connections"].array.isEmpty { Text("当前没有可展示的联网活动").foregroundStyle(.secondary).padding(.vertical, 24) }
            ScrollView { LazyVStack(spacing: 0) { ForEach(Array(store.activity["connections"].array.enumerated()), id: \.offset) { index, item in
                HStack { Text(item["process"].text).frame(width: 150, alignment: .leading); VStack(alignment: .leading) { Text(item["host"].text == "—" ? item["ip"].text : item["host"].text); Text(item["ip"].text + ":" + item["port"].text).font(.caption).foregroundStyle(.secondary) }; Spacer(); Text(item["protocol"].text).font(.caption); Text("↓ \(bytes(item["rx_bytes"]))  ↑ \(bytes(item["tx_bytes"]))").font(.caption.monospacedDigit()) }.padding(.vertical, 6)
                    .padding(.horizontal, 10).background(Color.primary.opacity(index.isMultiple(of: 2) ? 0.025 : 0), in: RoundedRectangle(cornerRadius: 7))
            } } }.frame(height: store.activity["connections"].array.isEmpty ? 0 : min(300, CGFloat(store.activity["connections"].array.count) * 48))
            Text("仅显示连接元数据，不读取通信内容。").font(.caption).foregroundStyle(.secondary)
        }
        Panel(title: "设备资料") { RawDetails(title: "展开设备详情（包含卡片标识）", value: store.status) }
    }
    func metric(_ title: String, _ value: String) -> some View { Panel(title: "") { Text(title).font(.caption).foregroundStyle(.secondary); Text(value).font(.title3.weight(.semibold)).lineLimit(2).textSelection(.enabled) }.frame(minHeight: 110) }
    func bytes(_ value: HubValue) -> String { guard let number = Int64(value.text) else { return "—" }; return ByteCountFormatter.string(fromByteCount: number, countStyle: .binary) }
    func traffic(_ title: String, _ key: String) -> some View { InfoPair(title: title, value: store.traffic["available"].bool ? bytes(store.traffic[key]) : "—") }
}
struct SMSComposer: View {
    @ObservedObject var store: HubStore
    @Environment(\.dismiss) private var dismiss
    @State private var recipient = ""
    @State private var message = ""
    @State private var sending = false
    @State private var error = ""
    var body: some View {
        SheetFrame(title: "新短信", subtitle: "通过当前 SIM 卡发送，运营商可能收取费用。") {
            HStack(spacing: 14) { Image(systemName: "person.crop.circle").foregroundStyle(.secondary); HubField(title: "收件人", text: $recipient, placeholder: "手机号，支持国际区号") }
            VStack(alignment: .leading, spacing: 7) {
                Text("短信内容").font(.system(size: 11, weight: .medium)).foregroundStyle(.secondary)
                ZStack(alignment: .topLeading) {
                    if message.isEmpty { Text("输入你想发送的消息…").font(.system(size: 13)).foregroundStyle(.tertiary).padding(.horizontal, 15).padding(.top, 12).allowsHitTesting(false) }
                    TextEditor(text: $message).font(.system(size: 13)).scrollContentBackground(.hidden).padding(8).accessibilityLabel("短信内容")
                }.frame(height: 180).background(Color.primary.opacity(0.025), in: RoundedRectangle(cornerRadius: 10))
            }
            if !error.isEmpty { Text(error).foregroundStyle(.red).font(.caption).textSelection(.enabled) }
            HStack { Text("\(message.count) 字 · 自动分片").font(.caption).foregroundStyle(.secondary); Spacer(); Button("取消") { dismiss() }.keyboardShortcut(.cancelAction).disabled(sending); Button(sending ? "发送中…" : "发送", systemImage: "arrow.up") {
                confirm("发送短信", "发送至 \(recipient)，运营商可能收取费用。") {
                    sending = true
                    Task {
                        do { _ = try await store.service.request("api/sms/send", method: "POST", body: ["phone": recipient, "message": message]); store.notifications.report("短信发送完成", success: true); await store.refresh(); dismiss() }
                        catch { self.error = error.localizedDescription }
                        sending = false
                    }
                }
            }.buttonStyle(.borderedProminent).tint(HubStyle.accent).disabled(recipient.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || message.isEmpty || sending) }
        }.interactiveDismissDisabled(sending)
    }
}
struct PhonePage: View {
	@State private var showingHistory = false
    @ObservedObject var store: HubStore
    @ObservedObject var voice: NativeVoice
    @State private var number = ""
    @State private var useAudio = true
    @State private var advancedAudio = false
    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
        HStack {
            Label("拨号键盘", systemImage: "circle.grid.3x3.fill").font(.headline)
            Spacer()
            Button("最近通话", systemImage: "clock.arrow.circlepath") { showingHistory = true }
                .help("查看来电、去电与未接来电")
        }
        .sheet(isPresented: $showingHistory) {
            CommunicationHistoryView(store: store, kind: "call", onSelectNumber: { number = $0 })
        }
        HStack(alignment: .top, spacing: 18) {
        Panel(title: "拨号") {
            Image(systemName: "phone.circle.fill").font(.system(size: 42, weight: .light)).foregroundStyle(HubStyle.accent.opacity(0.8)).frame(maxWidth: .infinity).padding(.top, 6)
            HubField(title: "电话号码", text: $number, placeholder: "+64 …")
            Text(store.calls.isEmpty ? "准备就绪，等待拨号" : store.calls.map { "\($0["number"].text) · \(callState($0["state"].text))" }.joined(separator: "\n")).font(.caption).foregroundStyle(.secondary).frame(maxWidth: .infinity)
            LazyVGrid(columns: Array(repeating: GridItem(.flexible()), count: 3), spacing: 10) {
                ForEach(Array("123456789*0#").map(String.init), id: \.self) { digit in
                    Button {
                        if store.calls.contains(where: { $0["state"].text == "0" }) { store.action("api/calls", body: ["action": "dtmf", "digit": digit]) }
                        else if store.calls.isEmpty { number += digit }
                    } label: { Text(digit).font(.system(size: 24, weight: .regular, design: .rounded)).frame(maxWidth: .infinity).frame(height: 48).background(Color.primary.opacity(0.035), in: RoundedRectangle(cornerRadius: 12)).contentShape(Rectangle()) }.buttonStyle(.plain)
                }
            }.padding(.vertical, 10)
            HStack {
                Button("拨打", systemImage: "phone.fill") { confirm("拨打 \(number)", useAudio ? "将连接电脑麦克风与模块。首次初始化可能短暂中断 USB 网络；运营商可能收取费用。" : "仅操作模块拨号，不连接电脑音频。") { store.phone("dial", number: number, audio: useAudio) } }.disabled(number.range(of: "^\\+?[0-9]{1,20}$", options: .regularExpression) == nil || !store.calls.isEmpty)
                    .buttonStyle(.borderedProminent).tint(HubStyle.accent)
                Button("接听") { store.phone("answer", audio: useAudio) }.disabled(!store.calls.contains { ["4", "5"].contains($0["state"].text) })
                Button("挂断", role: .destructive) { store.phone("hangup", audio: false) }.disabled(store.calls.isEmpty)
                Spacer(minLength: 0)
                Button {
                    guard !number.isEmpty, store.calls.isEmpty else { return }
                    number.removeLast()
                } label: { Image(systemName: "delete.left").frame(width: 16, height: 16) }
                    .buttonStyle(ToolbarButtonStyle())
                    .disabled(number.isEmpty || !store.calls.isEmpty)
                    .help("删除号码最后一位").accessibilityLabel("删除号码最后一位")
            }
        }.frame(width: 300)
        VStack(spacing: 16) {
        Panel(title: "通话音频") {
            SettingRow(title: "启动后后台准备", detail: "加载模块驱动并待机，不开启麦克风；USB 可能短暂重连") { Toggle("后台准备模块音频", isOn: $store.backgroundAudio).labelsHidden().toggleStyle(.switch).disabled(voice.connected || !store.calls.isEmpty) }
            SettingRow(title: "自动连接", detail: "拨号时使用电脑音频") { Toggle("自动使用电脑通话音频", isOn: $useAudio).labelsHidden().toggleStyle(.switch).controlSize(.small) }
            Divider()
            devicePicker("麦克风", $voice.mic, voice.devices.filter { $0.input && !$0.module })
            devicePicker("扬声器", $voice.speaker, voice.devices.filter { $0.output && !$0.module })
            HStack { Image(systemName: "speaker.wave.2").foregroundStyle(.secondary); Slider(value: $voice.volume, in: 0...1).accessibilityLabel("收听音量"); Text("\(Int(voice.volume * 100))%").font(.caption.monospacedDigit()).foregroundStyle(.secondary).frame(width: 32) }
            HStack { Text("麦克风静音").font(.system(size: 12)); Spacer(); Toggle("麦克风静音", isOn: $voice.muted).labelsHidden().toggleStyle(.switch).controlSize(.small).disabled(!voice.connected) }
        }
        Panel(title: "模块音频") {
            HStack { StatusPill(text: voice.connected ? "音频已连接" : "音频未连接", active: voice.connected); Spacer(); Button { voice.refreshDevices() } label: { Image(systemName: "arrow.clockwise") }.help("查找音频设备") }
            Button("设备与高级控制…", systemImage: "slider.horizontal.3") { advancedAudio = true }
            Text(voice.status).font(.caption).foregroundStyle(.secondary)
        }
        .sheet(isPresented: $advancedAudio) {
            SheetFrame(title: "模块音频", subtitle: "通常由拨号自动连接。仅在排查设备时使用以下控制。") {
                VStack(alignment: .leading, spacing: 12) {
                    devicePicker("模块输入", $voice.moduleInput, voice.devices.filter { $0.input && $0.module })
                    devicePicker("模块输出", $voice.moduleOutput, voice.devices.filter { $0.output && $0.module })
            HStack {
                Button("连接音频") { confirm("连接原生音频", "会使用麦克风并临时启用模块音频，USB 网络可能短暂重连；不保存录音。") { store.run { try await voice.connect(store.service) } } }.disabled(voice.busy)
                Button("断开音频") { voice.stopStreams() }
            }
                    Button("停止待机并恢复 USB") { Task { await voice.release() } }.disabled(voice.busy)
                }.padding(.top, 10)
                Text(voice.status).font(.caption).foregroundStyle(.secondary)
                Divider()
                HStack { Spacer(); Button("完成") { advancedAudio = false }.keyboardShortcut(.cancelAction) }
            }
        }
        Label("建议佩戴耳机。原生音频仍需真机验收。", systemImage: "headphones").font(.system(size: 11)).foregroundStyle(.secondary).frame(maxWidth: .infinity, alignment: .leading).padding(.horizontal, 6)
        }
        }
        }
    }
    func devicePicker(_ title: String, _ selection: Binding<String>, _ choices: [AudioDevice]) -> some View {
        SettingRow(title: title) {
            HubSelect(title: "", selection: selection, options: choices.map { HubOption(id: $0.id, title: $0.name) }, empty: "未检测到设备")
                .frame(maxWidth: 310).disabled(voice.connected || voice.busy).accessibilityLabel(title)
        }
    }
    func callState(_ state: String) -> String { ["0": "通话中", "1": "保持", "2": "拨号中", "3": "响铃中", "4": "来电", "5": "呼叫等待"][state] ?? "未知状态" }
}
struct NetworkPage: View {
    @ObservedObject var store: HubStore
    @State private var apn = ""
    @State private var pdn = "IP"
    var body: some View {
        Panel(title: "网络诊断") {
            HStack { Label(store.network["usb_network_ready"].bool ? "USB 网卡已就绪" : "USB 网卡未就绪", systemImage: "network"); Spacer(); Button("启用并获取 IP") { confirm("启用网络服务", "会请求 macOS 管理员授权并重新申请 DHCP。") { store.action("api/network/enable-service") } } }
            VStack(spacing: 0) {
                DetailRow(title: "网络服务", value: store.network["network_service"]["name"].text)
                DetailRow(title: "默认出口", value: store.network["default_route"]["interface"].text + " · " + store.network["default_route"]["gateway"].text)
                DetailRow(title: "蜂窝 IP", value: store.network["pdp_addresses"].array.map(\.text).joined(separator: " · "))
                DetailRow(title: "数据会话", value: store.network["active_contexts"].array.map { "CID " + $0.text }.joined(separator: " · "))
            }
            Divider()
            InterfaceTable(values: store.network["mac_interfaces"].array)
            HStack { check("检测 4G 出口", "api/network/check-4g"); check("检测代理", "api/network/check-proxy") }
            Text(store.diagnostic).font(.system(.caption, design: .monospaced)).textSelection(.enabled)
        }
        Panel(title: "USB 工作模式") {
            HStack { ForEach(0..<4) { mode in Button(["短信 / 管理", "4G 网卡", "实验模式 2", "实验模式 3"][mode]) { confirm("切换 USB 模式", "可能需要重启模块，会中断当前连接。") { store.action("api/network/usbnet", body: ["mode": mode]) } } }; Spacer(); Button("重启模块", role: .destructive) { confirm("重启模块", "将中断短信、通话和网络连接。") { store.action("api/network/reboot-module") } } }
        }
        Panel(title: "主数据 APN") {
            HStack(alignment: .top, spacing: 16) {
                HubField(title: "接入点名称", text: $apn, placeholder: "输入运营商提供的 APN")
                APNProtocolPicker(selection: $pdn).frame(width: 250)
            }
            HStack { Text("预设").font(.caption).foregroundStyle(.secondary); Button("One NZ · web") { apn = "web" }; Spacer(); Button("保存 APN") { confirm("保存 APN", "主数据 APN 将改为 \(apn)，下一次数据连接生效。") { store.action("api/network/apn", body: ["apn": apn, "pdn": pdn]) } }.disabled(apn.isEmpty) }
        }
    }
    func check(_ title: String, _ path: String) -> some View { Button(title) { store.run { let result = try await store.service.request(path, method: "POST"); store.diagnostic = result["summary"].text + "\n" + result["detail"].text } } }
}
struct ATPage: View {
    @ObservedObject var store: HubStore
    @State private var command = ""
    var body: some View {
        Panel(title: "高级诊断") {
            Text("命令直接发送至模块。修改配置或重启命令可能中断当前连接。").font(.caption).foregroundStyle(.secondary)
            HStack { Text("›").foregroundStyle(.green); HubField(title: "", text: $command, placeholder: "输入 AT 命令"); Button("执行") { confirm("执行 AT 命令", command) { store.run { store.console = try await store.service.request("api/at", method: "POST", body: ["command": command])["response"].text } } }.disabled(!command.uppercased().hasPrefix("AT")) }
            Divider(); Text(store.console).font(.system(.body, design: .monospaced)).textSelection(.enabled).frame(maxWidth: .infinity, minHeight: 280, alignment: .topLeading)
        }
    }
}
struct SettingsPage: View {
    @ObservedObject var service: HubService
    @ObservedObject var notifications: HubNotifications
    var body: some View {
        HistoryBackupSettings(service: service)
        SettingsSection(title: "通知与操作结果") {
            SettingRow(title: "来电与短信提示音", detail: "使用系统通知声音；受系统音量和专注模式控制") { Toggle("提示音", isOn: $notifications.incomingSound).labelsHidden() }
            SettingRow(title: "macOS 系统通知", detail: notifications.permission) {
                Button("授权通知") { Task { await notifications.authorize() } }
            }
            Divider().padding(.vertical, 6)
            DetailRow(title: "最近结果", value: notifications.latest)
        }
        SettingsSection(title: "客户端与 Web") {
            DetailRow(title: "连接状态", value: service.connectionText)
            DetailRow(title: "服务地址", value: service.base.absoluteString)
            DetailRow(title: "运行方式", value: service.ownsService ? "由客户端管理" : "复用独立运行的服务")
            Divider().padding(.vertical, 6)
            SettingRow(title: "Web 控制台", detail: "共享设备服务，保留完整功能") { Button("打开 Web", systemImage: "arrow.up.right") { service.openWeb() } }
            Divider().padding(.vertical, 6)
            SettingRow(title: "运行日志", detail: "查看本地启动与运行记录") { Button("打开目录", systemImage: "folder") { NSWorkspace.shared.open(FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Library/Logs/DJ 4G Hub")) } }
        }
        SettingsSection(title: "实验音频") {
            DetailRow(title: "驱动来源", value: "本机配置，不自动下载")
            DetailRow(title: "音频会话", value: "Web 与客户端不可同时占用模块")
            Divider().padding(.vertical, 8)
            Text("退出时关闭客户端音频，只停止客户端自身启动的服务。模块初始化失败仍需检查硬件连接。").font(.system(size: 11)).foregroundStyle(.secondary).padding(.bottom, 6)
        }
    }
}
