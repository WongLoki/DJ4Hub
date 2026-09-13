import SwiftUI
import AppKit

struct HubField: View {
    let title: String
    @Binding var text: String
    var placeholder = ""
    var secure = false
    @FocusState private var focused: Bool
    var body: some View {
        VStack(alignment: .leading, spacing: 7) {
            if !title.isEmpty { Text(title).font(.system(size: 11, weight: .medium)).foregroundStyle(.secondary) }
            Group {
                if secure { SecureField(placeholder, text: $text) }
                else { TextField(placeholder, text: $text) }
            }.textFieldStyle(.plain).font(.system(size: 13)).focused($focused)
                .padding(.horizontal, 12).frame(height: 44)
                .background(Color.primary.opacity(0.035), in: RoundedRectangle(cornerRadius: 10))
                .overlay(RoundedRectangle(cornerRadius: 10).stroke(focused ? HubStyle.accent.opacity(0.7) : Color.primary.opacity(0.065), lineWidth: 1).allowsHitTesting(false))
                .contentShape(Rectangle()).simultaneousGesture(TapGesture().onEnded { focused = true })
                .accessibilityLabel(title.isEmpty ? placeholder : title)
        }.frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct HubOption: Identifiable {
    let id: String
    let title: String
}

struct APNProtocolPicker: View {
    @Binding var selection: String
    private let options = [HubOption(id: "IP", title: "IPv4"), HubOption(id: "IPV4V6", title: "双栈"), HubOption(id: "IPV6", title: "IPv6")]
    var body: some View {
        VStack(alignment: .leading, spacing: 7) {
            Text("协议类型").font(.system(size: 11, weight: .medium)).foregroundStyle(.secondary)
            HStack(spacing: 3) {
                ForEach(options) { option in
                    Button { selection = option.id } label: {
                        Text(option.title).font(.system(size: 12, weight: selection == option.id ? .semibold : .regular))
                            .frame(maxWidth: .infinity, minHeight: 36)
                            .foregroundStyle(selection == option.id ? HubStyle.accent : Color.secondary)
                            .background(selection == option.id ? HubStyle.accent.opacity(0.12) : .clear, in: RoundedRectangle(cornerRadius: 7))
                            .contentShape(Rectangle())
                    }.buttonStyle(.plain)
                        .accessibilityLabel(option.id == "IPV4V6" ? "IPv4 与 IPv6 双栈" : option.title)
                        .accessibilityAddTraits(selection == option.id ? .isSelected : [])
                        .help(option.id == "IPV4V6" ? "同时使用 IPv4 和 IPv6；保存 APN 后才会应用" : "使用 \(option.title)；保存 APN 后才会应用")
                }
            }.padding(4).frame(height: 44)
                .background(Color.primary.opacity(0.035), in: RoundedRectangle(cornerRadius: 10))
                .overlay(RoundedRectangle(cornerRadius: 10).stroke(Color.primary.opacity(0.065)).allowsHitTesting(false))
        }
    }
}


/// AppKit owns menu tracking, keyboard navigation and hit testing across the full control.
struct HubSelect: View {
    let title: String
    @Binding var selection: String
    let options: [HubOption]
    var empty = "选择选项"
    var body: some View {
        VStack(alignment: .leading, spacing: 7) {
            if !title.isEmpty { Text(title).font(.system(size: 11, weight: .medium)).foregroundStyle(.secondary) }
            NativeChoice(selection: $selection, options: options, placeholder: empty, title: title).frame(height: 36)
        }.frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct NativeChoice: NSViewRepresentable {
    @Environment(\.isEnabled) private var enabled
    @Binding var selection: String
    let options: [HubOption]
    let placeholder: String
    let title: String
    class PopUp: NSPopUpButton {
        override func acceptsFirstMouse(for event: NSEvent?) -> Bool { true }
        override var intrinsicContentSize: NSSize { NSSize(width: NSView.noIntrinsicMetric, height: 36) }
    }
    class Coordinator: NSObject {
        var parent: NativeChoice
        init(_ parent: NativeChoice) { self.parent = parent }
        @objc func changed(_ sender: NSPopUpButton) {
            guard let id = sender.selectedItem?.representedObject as? String, parent.options.contains(where: { $0.id == id }) else { return }
            parent.selection = id
        }
    }
    func makeCoordinator() -> Coordinator { Coordinator(self) }
    func makeNSView(context: Context) -> PopUp {
        let view = PopUp(frame: .zero, pullsDown: false)
        view.controlSize = .large
        view.font = .systemFont(ofSize: 12)
        view.target = context.coordinator
        view.action = #selector(Coordinator.changed(_:))
        view.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
        return view
    }
    func updateNSView(_ view: PopUp, context: Context) {
        context.coordinator.parent = self
        let valid = options.contains { $0.id == selection }
        let entries = (valid ? [] : [HubOption(id: "", title: placeholder)]) + options
        if view.itemArray.map({ $0.representedObject as? String ?? "" }) != entries.map(\.id) || view.itemTitles != entries.map(\.title) {
            let menu = NSMenu()
            menu.autoenablesItems = false
            for option in entries {
                let item = NSMenuItem(title: option.title, action: nil, keyEquivalent: "")
                item.representedObject = option.id
                menu.addItem(item)
            }
            view.menu = menu
        }
        if let index = entries.firstIndex(where: { $0.id == selection }) { view.selectItem(at: index) }
        else { view.selectItem(at: 0) }
        view.isEnabled = enabled && !options.isEmpty
        view.setAccessibilityLabel(title.isEmpty ? placeholder : title)
        view.toolTip = options.first { $0.id == selection }?.title ?? placeholder
    }
}

struct DetailRow: View {
    let title: String
    let value: String
    var body: some View {
        HStack(alignment: .top, spacing: 20) {
            Text(title).font(.system(size: 12)).foregroundStyle(.secondary).frame(width: 115, alignment: .leading)
            Text(value.isEmpty ? "未提供" : value).font(.system(size: 13, weight: .medium)).textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading)
        }.padding(.vertical, 7)
    }
}

struct EmptyState: View {
    let symbol: String
    let title: String
    let detail: String
    var body: some View {
        HStack(spacing: 16) {
            Image(systemName: symbol).font(.system(size: 24, weight: .light)).foregroundStyle(HubStyle.accent).frame(width: 44, height: 44).background(HubStyle.accent.opacity(0.07), in: RoundedRectangle(cornerRadius: 12))
            VStack(alignment: .leading, spacing: 6) { Text(title).font(.system(size: 14, weight: .semibold)); Text(detail).font(.system(size: 12)).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true) }
            Spacer(minLength: 0)
        }.padding(.vertical, 12)
    }
}

struct RecordDetails: View {
    let value: HubValue
    private var keys: [String] { (value.raw as? [String: Any])?.keys.sorted() ?? [] }
    private let labels = ["card_type": "卡片类型", "message": "说明", "firmware": "固件", "operator": "运营商", "sim_inserted": "SIM 已插入", "imei": "IMEI", "iccid": "ICCID", "imsi": "IMSI", "phone_number": "手机号码", "chip_info": "芯片信息", "sku_name": "卡片型号", "serial_number": "序列号", "eids": "EID 列表", "eid": "EID", "aid": "AID", "aid_hex": "AID", "name": "名称", "state_text": "状态", "service_provider_name": "服务商", "signal_dbm": "信号 / dBm", "network_mode": "网络制式", "reg_status_text": "注册状态", "profiles": "Profile 列表", "raw": "诊断原始字段", "errors": "诊断异常"]
    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            ForEach(keys, id: \.self) { key in
                if value[key].raw is [String: Any] || value[key].raw is [Any] {
                    DisclosureGroup(labels[key] ?? key) { nested(value[key]).padding(.leading, 8) }.font(.system(size: 12)).padding(.vertical, 7)
                } else { DetailRow(title: labels[key] ?? key, value: value[key].text) }
            }
            if keys.isEmpty { Text("暂无详细资料").font(.caption).foregroundStyle(.secondary) }
        }
    }
    private func nested(_ item: HubValue) -> AnyView {
        if item.raw is [String: Any] { return AnyView(RecordDetails(value: item)) }
        return AnyView(VStack(alignment: .leading, spacing: 8) {
            ForEach(Array(item.array.enumerated()), id: \.offset) { index, entry in
                if entry.raw is [String: Any] { DisclosureGroup("记录 \(index + 1)") { RecordDetails(value: entry) } }
                else { DetailRow(title: "\(index + 1)", value: entry.text) }
            }
        })
    }
}
