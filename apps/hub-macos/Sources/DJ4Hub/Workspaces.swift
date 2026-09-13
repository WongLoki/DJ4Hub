import SwiftUI
import AppKit

/// A focused task sheet keeps editable drafts separate from live device polling.
struct SheetFrame<Content: View>: View {
    let title: String
    let subtitle: String
    @ViewBuilder var content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            VStack(alignment: .leading, spacing: 5) {
                Text(title).font(.system(size: 20, weight: .semibold))
                Text(subtitle).font(.callout).foregroundStyle(.secondary)
            }
            Divider()
            content
        }.padding(24).frame(width: 560).background(Color(nsColor: .windowBackgroundColor))
            .buttonStyle(HubButtonStyle()).disclosureGroupStyle(HubDisclosureStyle()).tint(HubStyle.accent)
    }
}

struct SettingRow<Content: View>: View {
    let title: String
    var detail = ""
    @ViewBuilder var content: Content
    var body: some View {
        HStack(spacing: 16) {
            VStack(alignment: .leading, spacing: 4) {
                Text(title).font(.system(size: 12, weight: .medium))
                if !detail.isEmpty { Text(detail).font(.system(size: 11)).foregroundStyle(.secondary) }
            }.frame(minWidth: 85, alignment: .leading)
            Spacer(minLength: 8)
            content
        }.frame(minHeight: 44).padding(.vertical, 3)
    }
}

struct SettingsSection<Content: View>: View {
    let title: String
    @ViewBuilder var content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title).font(.system(size: 12, weight: .semibold)).foregroundStyle(.secondary).padding(.horizontal, 4)
            VStack(alignment: .leading, spacing: 0) { content }
                .padding(.horizontal, 16).padding(.vertical, 8)
                .background(Color(nsColor: .controlBackgroundColor), in: RoundedRectangle(cornerRadius: 10))
                .overlay(RoundedRectangle(cornerRadius: 10).stroke(Color.primary.opacity(0.06)).allowsHitTesting(false))
        }
    }
}

struct SMSRecord: Identifiable {
    let value: HubValue
    var id: String { [value["sender"].text, value["timestamp"].text, value["content"].text].joined(separator: "\u{0}") }
}

struct SMSPage: View {
	@State private var showingHistory = false
    @ObservedObject var store: HubStore
    @State private var selection: String?
    @State private var query = ""
    @State private var composing = false
    private var records: [SMSRecord] {
        var seen = Set<String>()
        return store.messages.map { SMSRecord(value: $0) }.filter {
            seen.insert($0.id).inserted && (query.isEmpty || $0.value["sender"].text.localizedCaseInsensitiveContains(query) || $0.value["content"].text.localizedCaseInsensitiveContains(query))
        }
    }
    private var selected: HubValue? { records.first { $0.id == selection }?.value }
    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 8) {
                Text("收件箱").font(.headline)
                Button("按卡记录") { showingHistory = true }.sheet(isPresented: $showingHistory) { CommunicationHistoryView(store: store, kind: "sms") }
                Text("\(store.messages.count) 条").font(.caption).foregroundStyle(.secondary)
                Spacer()
                HStack(spacing: 6) { Image(systemName: "magnifyingglass").foregroundStyle(.secondary); TextField("搜索短信", text: $query).textFieldStyle(.plain).accessibilityLabel("搜索短信") }.font(.system(size: 12)).padding(.horizontal, 10).frame(width: 170, height: 34).background(Color.primary.opacity(0.035), in: RoundedRectangle(cornerRadius: 7))
                Button("读取模块", systemImage: "arrow.clockwise") { store.action("api/sms/refresh") }.buttonStyle(ToolbarButtonStyle())
                Menu { Button("清理模块旧短信", role: .destructive) { confirm("清理模块短信", "该操作会删除模块中保存的旧短信，无法撤销。") { store.action("api/sms/clear-module") } } } label: { Image(systemName: "ellipsis").frame(width: 34, height: 34).contentShape(Rectangle()) }.menuStyle(.borderlessButton).menuIndicator(.hidden).frame(width: 34, height: 34).contentShape(Rectangle()).background(Color.primary.opacity(0.04), in: RoundedRectangle(cornerRadius: 7)).help("更多短信操作").accessibilityLabel("更多短信操作")
                Button("新短信", systemImage: "square.and.pencil") { composing = true }.buttonStyle(ToolbarButtonStyle(prominent: true))
            }.padding(14)
            Divider()
            HSplitView {
                ScrollViewReader { scroll in
                List(selection: $selection) {
                    ForEach(records) { record in
                        VStack(alignment: .leading, spacing: 6) {
                            Text(record.value["sender"].text).font(.system(size: 13, weight: .semibold)).lineLimit(1)
                            Text(record.value["content"].text).font(.system(size: 12)).lineLimit(2).fixedSize(horizontal: false, vertical: true).foregroundStyle(.secondary)
                            Text(record.value["timestamp"].text).font(.system(size: 10)).foregroundStyle(.tertiary)
                        }.padding(.vertical, 7).frame(maxWidth: .infinity, alignment: .leading).contentShape(Rectangle()).tag(record.id).id(record.id)
                    }
                }.listStyle(.inset)
                    // Wait for the inserted row to be laid out before aligning it.
                    // The selection is deliberately independent of scrolling.
                    .task(id: records.first?.id) {
                        guard let first = records.first?.id else { return }
                        await Task.yield()
                        guard !Task.isCancelled else { return }
                        scroll.scrollTo(first, anchor: .top)
                    }
                }.frame(minWidth: 230, idealWidth: 280, maxWidth: 350)
                ScrollView {
                    if let message = selected {
                        VStack(alignment: .leading, spacing: 18) {
                            Label(message["sender"].text, systemImage: "person.crop.circle").font(.title3.weight(.semibold))
                            Text(message["timestamp"].text).font(.caption).foregroundStyle(.secondary)
                            Divider()
                            Text(message["content"].text).font(.system(size: 14)).lineSpacing(5).textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading)
                            let code = message["code"].text
                            if !code.isEmpty && code != "—" {
                                Button("复制验证码 · \(code)", systemImage: "doc.on.doc") { NSPasteboard.general.clearContents(); NSPasteboard.general.setString(code, forType: .string) }
                            }
                        }.padding(24)
                    } else {
                        EmptyState(symbol: "tray", title: records.isEmpty ? "没有短信" : "选择一条短信", detail: records.isEmpty ? "读取模块或调整搜索条件。" : "左侧浏览消息，右侧查看完整内容与验证码。").padding(24)
                    }
                }.frame(minWidth: 270, maxWidth: .infinity)
            }.frame(height: 440)
        }.background(Color(nsColor: .controlBackgroundColor)).clipShape(RoundedRectangle(cornerRadius: 12))
            .overlay(RoundedRectangle(cornerRadius: 12).stroke(Color.primary.opacity(0.07)).allowsHitTesting(false))
            .sheet(isPresented: $composing) { SMSComposer(store: store) }
    }
}

struct InterfaceRecord: Identifiable {
    let name: String
    let address: String
    let kind: String
    let state: String
    var id: String { name }
    init(_ value: HubValue) {
        name = value["name"].text
        address = value["ipv4"].text.isEmpty ? "—" : value["ipv4"].text
        kind = value["kind"].text
        state = value["status"].text == "active" ? "活跃" : "未活跃"
    }
}

struct InterfaceTable: View {
    let values: [HubValue]
    @State private var showInactive = false
    @State private var selection: String?
    @State private var sortOrder = [KeyPathComparator(\InterfaceRecord.name)]
    var rows: [InterfaceRecord] { values.filter { showInactive || $0["status"].text == "active" }.map(InterfaceRecord.init).sorted(using: sortOrder) }
    var body: some View {
        VStack(spacing: 10) {
            HStack {
                Text("网络接口").font(.system(size: 12, weight: .semibold))
                Text("\(rows.count) / \(values.count)").font(.caption).foregroundStyle(.secondary)
                Spacer()
                Toggle("包含未活跃接口", isOn: $showInactive).toggleStyle(.checkbox).font(.caption)
            }
            Table(rows, selection: $selection, sortOrder: $sortOrder) {
                TableColumn("接口", value: \.name).width(min: 65, ideal: 100)
                TableColumn("IPv4 地址", value: \.address)
                TableColumn("类型", value: \.kind)
                TableColumn("状态", value: \.state) { row in Text(row.state).foregroundStyle(row.state == "活跃" ? HubStyle.accent : .secondary) }.width(min: 60, ideal: 90)
            }.tableStyle(.inset(alternatesRowBackgrounds: true)).frame(height: 220)
                .overlay { if rows.isEmpty { Text("暂无活跃接口").font(.caption).foregroundStyle(.secondary).allowsHitTesting(false) } }
        }
    }
}
