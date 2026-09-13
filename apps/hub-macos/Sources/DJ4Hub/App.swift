import SwiftUI
import AppKit

@MainActor final class HubDelegate: NSObject, NSApplicationDelegate {
    var store: HubStore?
    private var finishing = false
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        if finishing { return .terminateNow }
        finishing = true
        store?.stopBackgroundPreparation(shutdown: true)
        store?.voice.stopStreams()
        Task {
            await store?.voice.release()
            store?.service.stopOwned()
            sender.reply(toApplicationShouldTerminate: true)
        }
        return .terminateLater
    }
}
@main struct DJ4HubApp: App {
    @NSApplicationDelegateAdaptor(HubDelegate.self) var delegate
    @StateObject private var store = HubStore()
    var body: some Scene {
        WindowGroup("DJ 4G Hub", id: "main") {
            HubRoot(store: store, service: store.service)
                .background(UnifiedWindowAppearance())
                .onAppear { delegate.store = store }
        }.defaultSize(width: 1150, height: 800)
        .windowStyle(.hiddenTitleBar)
        .commands {
            CommandGroup(replacing: .newItem) {}
            CommandGroup(after: .appInfo) { Button("打开 Web 控制台") { store.service.openWeb() } }
        }
        MenuBarExtra("DJ 4G Hub", systemImage: "antenna.radiowaves.left.and.right") {
            Button("显示客户端") { NSApp.activate(ignoringOtherApps: true); NSApp.windows.first?.makeKeyAndOrderFront(nil) }
            Button("打开 Web") { store.service.openWeb() }
            Divider()
            Button("退出") { NSApp.terminate(nil) }
        }
    }
}

/// Configure only the owning window; never alter sheets or other app windows.
struct UnifiedWindowAppearance: NSViewRepresentable {
    final class WindowView: NSView {
        override func viewDidMoveToWindow() {
            super.viewDidMoveToWindow()
            guard let window else { return }
            window.titleVisibility = .hidden
            window.titlebarAppearsTransparent = true
            window.titlebarSeparatorStyle = .none
            window.styleMask.insert(.fullSizeContentView)
        }
    }
    func makeNSView(context: Context) -> WindowView { WindowView() }
    func updateNSView(_ nsView: WindowView, context: Context) {}
}
