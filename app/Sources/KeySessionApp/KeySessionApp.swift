import AppKit
import KeySessionKit
import SwiftUI

extension Notification.Name {
    static let openKeySession = Notification.Name("openKeySession")
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    let visibility = AppVisibilitySettings()

    func applicationDidFinishLaunching(_ notification: Notification) {
        visibility.applyActivationPolicy()
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
    @State private var updates = AppUpdateController()

    var body: some Scene {
        Window("Key Session", id: "command-center") {
            CommandCenterView(model: model, updates: updates, visibility: appDelegate.visibility)
                .task {
                    model.startPolling()
                    updates.start()
                }
        }
        .defaultSize(width: 1180, height: 760)
        .windowResizability(.contentMinSize)
        .commands {
            CommandGroup(replacing: .appSettings) {
                Button("Settings…") {
                    model.selection = .settings
                    NotificationCenter.default.post(name: .openKeySession, object: nil)
                }
                .keyboardShortcut(",", modifiers: .command)
            }
            CommandGroup(after: .appInfo) {
                Button(updates.buttonTitle) { updates.checkForUpdates() }
                    .disabled(!updates.canCheckForUpdates)
            }
        }

        MenuBarExtra(isInserted: Binding(
            get: { appDelegate.visibility.showsMenuBar },
            set: { appDelegate.visibility.setMenuBarVisible($0) }
        )) {
            MenuBarSummaryView(model: model)
        } label: {
            MenuBarStatusLabel(model: model)
        }
        .menuBarExtraStyle(.window)
    }
}
