import SwiftUI

enum HistoryPresentation {
    static func date(_ value: String) -> Date? {
        let parser = ISO8601DateFormatter()
        parser.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return parser.date(from: value) ?? ISO8601DateFormatter().date(from: value)
    }
    static func day(_ value: String) -> String {
        guard let date = date(value) else { return "日期未知" }
        if Calendar.current.isDateInToday(date) { return "今天" }
        if Calendar.current.isDateInYesterday(date) { return "昨天" }
        return date.formatted(.dateTime.year().month().day())
    }
    static func time(_ value: String) -> String {
        date(value)?.formatted(date: .omitted, time: .shortened) ?? "—"
    }
    static func duration(_ seconds: Int) -> String {
        let seconds = max(0, seconds)
        return seconds < 60 ? "\(seconds) 秒" : "\(seconds / 60) 分 \(seconds % 60) 秒"
    }
    static func cardLabel(_ value: String) -> String {
        value.isEmpty || value == "—" ? "未归属" : "SIM · \(value.suffix(4))"
    }
    static func dialNumber(_ value: String) -> String? {
        let number = value.filter { !$0.isWhitespace && $0 != "-" && $0 != "(" && $0 != ")" }
        return number.range(of: "^\\+?[0-9]{1,20}$", options: .regularExpression) == nil ? nil : number
    }
}

struct CommunicationHistoryView: View {
    @ObservedObject var store: HubStore
    let kind: String
    var onSelectNumber: ((String) -> Void)? = nil
    @Environment(\.dismiss) private var dismiss
    @State private var card = "current"
    @State private var cards: [String: String] = [:]
    @State private var records: [HubValue] = []
    @State private var total = 0
    @State private var error = ""
    @State private var loading = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack {
                VStack(alignment: .leading, spacing: 4) {
                    Text(kind == "sms" ? "短信记录" : "最近通话").font(.title2.bold())
                    Text("\(total) 条记录 · 保存在这台 Mac 上").font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
                Button("完成") { dismiss() }.keyboardShortcut(.cancelAction)
            }.padding(20)
            HStack(spacing: 12) {
                HubSelect(title: "", selection: $card, options: cardOptions)
                    .frame(width: 240).accessibilityLabel("筛选 SIM 卡").disabled(loading)
                Spacer()
                if loading { ProgressView().controlSize(.small) }
                Button { Task { await load() } } label: { Image(systemName: "arrow.clockwise").frame(width: 20, height: 20) }
                    .buttonStyle(ToolbarButtonStyle()).disabled(loading).help("刷新记录").accessibilityLabel("刷新记录")
            }.padding(.horizontal, 20).padding(.bottom, 14)
            Divider()
            if !error.isEmpty {
                HStack(alignment: .top, spacing: 12) {
                    Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(.orange)
                    VStack(alignment: .leading, spacing: 4) {
                        Text("记录暂时无法读取").font(.headline)
                        Text(error).font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
                    }
                    Spacer()
                    Button("重试") { Task { await load() } }.disabled(loading)
                }.padding(18).background(Color.orange.opacity(0.06))
            }
            if records.isEmpty {
                VStack(spacing: 12) {
                    Image(systemName: "clock").font(.system(size: 36, weight: .light)).foregroundStyle(.secondary)
                    Text(loading ? "正在读取记录…" : (error.isEmpty ? "暂无记录" : "请检查本地服务后重试")).font(.headline)
                    if error.isEmpty && !loading {
                        Text(kind == "sms" ? "这张卡的短信记录会显示在这里。" : "这张卡的来电、去电和未接来电会显示在这里。")
                            .font(.callout).foregroundStyle(.secondary)
                    }
                }.frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                ScrollView {
                    LazyVStack(spacing: 0) {
                        ForEach(records.indices, id: \.self) { index in
                            let row = records[index]
                            let day = HistoryPresentation.day(row["started"].text)
                            if index == 0 || day != HistoryPresentation.day(records[index - 1]["started"].text) {
                                Text(day).font(.caption.weight(.semibold)).foregroundStyle(.secondary)
                                    .frame(maxWidth: .infinity, alignment: .leading).padding(.horizontal, 20).padding(.vertical, 10)
                                    .background(Color.primary.opacity(0.025))
                            }
                            historyRow(row)
                            Divider().padding(.leading, 66)
                        }
                        if records.count < total {
                            Button("加载更早记录（已显示 \(records.count) / \(total)）") { Task { await load(more: true) } }
                                .disabled(loading).padding(16)
                        }
                    }
                }
            }
            Divider()
            Text(kind == "sms" ? "来源未确认的模块旧短信保存在「未归属」。" : "时长为本机观测值，非运营商账单。选择号码只填入拨号框，不会立即呼叫。")
                .font(.caption).foregroundStyle(.secondary).padding(16)
        }.frame(width: 600, height: 620)
        .task { await load() }
        .onChange(of: card) { _ in
            records = []; total = 0; error = ""
            Task { await load() }
        }
    }
    private var cardOptions: [HubOption] {
        [HubOption(id: "current", title: "当前 SIM 卡"), HubOption(id: "all", title: "全部 SIM 卡"), HubOption(id: "unknown", title: "未归属")]
        + cards.keys.sorted().map { HubOption(id: $0, title: cards[$0] ?? HistoryPresentation.cardLabel($0)) }
    }
    private func historyRow(_ row: HubValue) -> some View {
        let missed = row["state"].text == "missed"
        let incoming = row["direction"].text == "incoming"
        let number = row["number"].text
        return HStack(alignment: .top, spacing: 14) {
            Image(systemName: kind == "sms" ? "message" : (incoming ? "phone.arrow.down.left" : "phone.arrow.up.right"))
                .font(.system(size: 17)).foregroundStyle(missed ? Color.red : Color.secondary)
                .frame(width: 30, height: 38)
            VStack(alignment: .leading, spacing: 6) {
                Text(number.isEmpty || number == "—" ? "未知号码" : number)
                    .font(.system(size: 16, weight: .semibold)).foregroundStyle(missed ? Color.red : Color.primary).textSelection(.enabled)
                Text(kind == "sms" ? label(row["state"].text) : "\(incoming ? "来电" : "去电") · \(label(row["state"].text))")
                    .font(.caption).foregroundStyle(missed ? Color.red : Color.secondary)
                if kind == "call" && row["state"].text == "ended" {
                    Text(HistoryPresentation.duration(Int(row["duration_seconds"].text) ?? 0)).font(.caption).foregroundStyle(.secondary)
                }
                Text(HistoryPresentation.cardLabel(row["iccid"].text)).font(.caption2).foregroundStyle(.secondary)
                if kind == "sms" { Text(row["content"].text).font(.callout).textSelection(.enabled) }
            }
            Spacer(minLength: 8)
            VStack(alignment: .trailing, spacing: 8) {
                Text(HistoryPresentation.time(row["started"].text)).font(.caption).foregroundStyle(.secondary).help(row["started"].text)
                if kind == "call", let dial = HistoryPresentation.dialNumber(number), let onSelectNumber {
                    Button { onSelectNumber(dial); dismiss() } label: {
                        Image(systemName: "phone").frame(width: 24, height: 24)
                    }.buttonStyle(ToolbarButtonStyle()).help("填入拨号框").accessibilityLabel("填入号码 \(number)")
                }
            }
        }.padding(.horizontal, 20).padding(.vertical, 14)
    }
    private func load(more: Bool = false) async {
        guard !loading else { return }
        loading = true; defer { loading = false }
        do {
            let result = try await store.service.request("api/history?kind=\(kind)&card=\(card)&offset=\(more ? records.count : 0)")
            cards = result["cards"].raw as? [String: String] ?? [:]
            records = more ? records + result["records"].array : result["records"].array
            total = Int(result["total"].text) ?? 0; error = ""
        } catch { self.error = error.localizedDescription }
    }
    private func label(_ value: String) -> String {
        ["received": "已接收", "sent": "已提交模块", "failed_or_partial": "失败或部分发送", "failed": "拨号失败", "observed": "呼叫中", "connected": "通话中", "ended": "已接通", "unanswered": "未接通", "missed": "未接", "interrupted": "记录中断"][value] ?? value
    }
}
