import SwiftUI

struct ProfileRecord: Identifiable {
    let aid: String
    let value: HubValue
    var id: String { aid + ":" + value["iccid"].text }
    var name: String { value["name"].text }
    var provider: String { value["service_provider_name"].text }
    var iccid: String { value["iccid"].text }
    var enabled: Bool { value["state"].text == "1" }
}

struct ESIMPage: View {
    @ObservedObject var store: HubStore
    @State private var selection: String?
    @State private var downloading = false
    @State private var renaming: ProfileRecord?
    private var profiles: [ProfileRecord] { store.esim["profiles"].array.flatMap { group in group["profiles"].array.map { ProfileRecord(aid: group["aid_hex"].text, value: $0) } } }
    private var selected: ProfileRecord? { profiles.first { $0.id == selection } }
    var body: some View {
        if store.esim["card_type"].text == "physical_sim" {
            Panel(title: "当前卡片") { EmptyState(symbol: "simcard", title: "实体 SIM 卡", detail: "此卡不支持 Profile 管理。短信、电话和蜂窝网络可在对应页面使用。") }
        } else if !(store.esim["profiles"].raw is [Any]) {
            Panel(title: "当前卡片") { EmptyState(symbol: "simcard", title: "等待卡片资料", detail: "识别到 eUICC 卡片后，将显示 Profile 列表与下载入口。") }
        } else {
            Panel(title: "") {
                HStack {
                    Label("已安装 Profile", systemImage: "simcard").font(.headline)
                    Text("\(profiles.count) 个").foregroundStyle(.secondary)
                    Spacer()
                    Button("下载 Profile", systemImage: "plus") { downloading = true }.buttonStyle(.borderedProminent)
                }
                Table(profiles, selection: $selection) {
                    TableColumn("名称") { Text($0.name).fontWeight(.medium) }
                    TableColumn("服务商") { Text($0.provider) }
                    TableColumn("ICCID") { Text($0.iccid).font(.system(.caption, design: .monospaced)) }
                    TableColumn("状态") { Text($0.enabled ? "已启用" : "未启用").foregroundStyle($0.enabled ? HubStyle.accent : .secondary) }.width(70)
                }.tableStyle(.inset(alternatesRowBackgrounds: true)).frame(height: 260)
                    .overlay { if profiles.isEmpty { Text("尚未安装 Profile").foregroundStyle(.secondary).allowsHitTesting(false) } }
                HStack {
                    Button("启用") {
                        guard let item = selected else { return }
                        confirm("启用 Profile", "将切换当前卡片，可能中断网络。") { store.action("api/esim/switch", body: ["iccid": item.iccid, "aid": item.aid]) }
                    }.disabled(selected == nil || selected?.enabled == true)
                    Button("改名…") { renaming = selected }.disabled(selected == nil)
                    Spacer()
                    Button("删除", role: .destructive) {
                        guard let item = selected else { return }
                        confirm("删除 Profile", "删除后无法撤销，可能需要运营商重新提供激活信息。") { store.action("api/esim/profile", method: "DELETE", body: ["iccid": item.iccid, "aid": item.aid]) }
                    }.disabled(selected == nil)
                }
            }.sheet(isPresented: $downloading) { ProfileEditor(store: store, profile: nil) }
                .sheet(item: $renaming) { ProfileEditor(store: store, profile: $0) }
            Panel(title: "卡片资料") { RawDetails(title: "查看芯片与卡片标识", value: store.esim) }
        }
    }
}

struct ProfileEditor: View {
    @ObservedObject var store: HubStore
    let profile: ProfileRecord?
    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @State private var smdp = ""
    @State private var matching = ""
    @State private var imei = ""
    @State private var code = ""
    @State private var aid = ""
    @State private var busy = false
    @State private var error = ""
    var body: some View {
        SheetFrame(title: profile == nil ? "下载 Profile" : "修改 Profile 名称", subtitle: profile == nil ? "使用运营商提供的激活信息，下载期间请勿拔出设备。" : "仅修改名称，不切换当前使用的 Profile。") {
            Group {
                if let profile {
                    Text(profile.iccid).font(.caption.monospaced()).foregroundStyle(.secondary)
                    HubField(title: "名称", text: $name, placeholder: "输入便于识别的名称")
                } else {
                    HubField(title: "SM-DP+ 地址", text: $smdp, placeholder: "运营商服务器地址")
                    HubField(title: "Matching ID", text: $matching, placeholder: "激活信息中的 Matching ID")
                    HubField(title: "设备 IMEI", text: $imei, placeholder: "15 位设备标识")
                    DisclosureGroup("可选参数") {
                        HubField(title: "确认码", text: $code, placeholder: "运营商要求时填写", secure: true)
                        HubField(title: "AID", text: $aid, placeholder: "不指定时留空")
                    }
                }
            }.disabled(busy)
            if !error.isEmpty { Text(error).font(.caption).foregroundStyle(.red).textSelection(.enabled) }
            Divider()
            HStack {
                if busy { ProgressView().controlSize(.small) }
                Spacer()
                Button("取消") { dismiss() }.keyboardShortcut(.cancelAction).disabled(busy)
                Button(busy ? "处理中…" : (profile == nil ? "下载并写入" : "保存名称")) {
                    if profile == nil { confirm("下载 Profile", "会向指定 SM-DP+ 服务器发送激活信息和 IMEI，并写入卡片。") { submit() } }
                    else { submit() }
                }.buttonStyle(.borderedProminent).disabled(busy || (profile == nil ? smdp.isEmpty || imei.isEmpty : name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty))
            }
        }.onAppear { name = profile?.name ?? "" }.interactiveDismissDisabled(busy)
    }
    private func submit() {
        busy = true; error = ""
        Task {
            do {
                if let profile { _ = try await store.service.request("api/esim/profile", method: "PATCH", body: ["iccid": profile.iccid, "aid": profile.aid, "name": name]) }
                else { _ = try await store.service.request("api/esim/download", method: "POST", body: ["smdp": smdp, "matching_id": matching, "confirmation_code": code, "imei": imei, "aid": aid]) }
                store.notifications.report(profile == nil ? "Profile 下载完成" : "Profile 名称已保存", success: true)
                await store.refresh(); dismiss()
            } catch { self.error = error.localizedDescription }
            busy = false
        }
    }
}
