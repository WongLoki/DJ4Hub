import SwiftUI

enum HubStyle {
    static let accent = Color(red: 0.10, green: 0.57, blue: 0.41)
    static let upload = Color(red: 0.46, green: 0.43, blue: 0.76)
    static let download = Color(red: 0.18, green: 0.64, blue: 0.70)
}

struct ToolbarButtonStyle: ButtonStyle {
    var prominent = false
    @Environment(\.isEnabled) private var enabled
    func makeBody(configuration: Configuration) -> some View {
        configuration.label.font(.system(size: 12, weight: .medium))
            .padding(.horizontal, 10).frame(minWidth: 34, minHeight: 34)
            .foregroundStyle(prominent ? Color.white : Color.primary)
            .background(prominent ? HubStyle.accent : Color.primary.opacity(configuration.isPressed ? 0.10 : 0.04), in: RoundedRectangle(cornerRadius: 7))
            .contentShape(Rectangle()).opacity(enabled ? (configuration.isPressed ? 0.75 : 1) : 0.4)
    }
}

struct HubBackdrop: View {
    @Environment(\.colorScheme) private var scheme
    var body: some View {
        ZStack {
            Color(nsColor: .windowBackgroundColor)
            LinearGradient(colors: [HubStyle.accent.opacity(scheme == .dark ? 0.035 : 0.045), .clear, HubStyle.upload.opacity(0.035)], startPoint: .topLeading, endPoint: .bottomTrailing)
        }.ignoresSafeArea()
    }
}

struct HubButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var enabled
    func makeBody(configuration: Configuration) -> some View {
        configuration.label.font(.system(size: 12, weight: .medium))
            .padding(.horizontal, 14).frame(minWidth: 40, minHeight: 40)
            .foregroundStyle(configuration.role == .destructive ? Color.red : Color.primary)
            .background(Color.primary.opacity(configuration.isPressed ? 0.12 : 0.045), in: RoundedRectangle(cornerRadius: 8))
            .contentShape(Rectangle())
            .opacity(enabled ? 1 : 0.38)
    }
}

struct HubDisclosureStyle: DisclosureGroupStyle {
    func makeBody(configuration: Configuration) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Button { configuration.isExpanded.toggle() } label: {
                HStack { configuration.label; Spacer(minLength: 16); Image(systemName: configuration.isExpanded ? "chevron.up" : "chevron.down").font(.system(size: 11, weight: .semibold)).foregroundStyle(.secondary) }
                    .font(.system(size: 12, weight: .medium)).padding(.horizontal, 12).frame(minHeight: 42)
                    .background(Color.primary.opacity(0.03), in: RoundedRectangle(cornerRadius: 9)).contentShape(Rectangle())
            }.buttonStyle(.plain).accessibilityValue(configuration.isExpanded ? "已展开" : "已折叠")
            if configuration.isExpanded { configuration.content }
        }
    }
}

struct StatusPill: View {
    let text: String
    var active = false
    var body: some View {
        HStack(spacing: 6) {
            Circle().fill(active ? HubStyle.accent : Color.secondary.opacity(0.5)).frame(width: 6, height: 6)
            Text(text).font(.system(size: 11, weight: .medium))
        }.padding(.horizontal, 10).padding(.vertical, 6)
            .background(Color.primary.opacity(0.045), in: Capsule())
    }
}

struct InfoPair: View {
    let title: String
    let value: String
    var body: some View {
        VStack(alignment: .leading, spacing: 7) {
            Text(title).font(.system(size: 10, weight: .medium)).foregroundStyle(.secondary)
            Text(value).font(.system(size: 14, weight: .semibold)).lineLimit(1).textSelection(.enabled)
        }.frame(maxWidth: .infinity, alignment: .leading)
    }
}
