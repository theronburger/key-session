import AppKit
import KeySessionKit
import SwiftUI

extension Notification.Name {
    static let openKeySession = Notification.Name("openKeySession")
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApplication.shared.setActivationPolicy(.regular)
        NSApplication.shared.activate(ignoringOtherApps: true)
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        if !flag { NotificationCenter.default.post(name: .openKeySession, object: nil) }
        sender.activate(ignoringOtherApps: true)
        return true
    }
}

@main
struct KeySessionApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @State private var model = AppModel()

    var body: some Scene {
        Window("Key Session", id: "command-center") {
            CommandCenterView(model: model)
                .task { model.startPolling() }
        }
        .defaultSize(width: 1180, height: 760)
        .windowResizability(.contentMinSize)

        MenuBarExtra {
            MenuBarSummaryView(model: model)
        } label: {
            MenuBarStatusLabel(model: model)
        }
        .menuBarExtraStyle(.window)

        Settings { SettingsView(model: model) }
    }
}
