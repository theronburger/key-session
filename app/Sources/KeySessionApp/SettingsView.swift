import KeySessionKit
import SwiftUI

struct SettingsView: View {
    @Bindable var model: AppModel
    @Bindable var updates: AppUpdateController
    @Bindable var visibility: AppVisibilitySettings

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 22) {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Settings").font(.largeTitle.weight(.bold))
                    Text("Control how Key Session appears and maintains its local installation.")
                        .font(.title3).foregroundStyle(.secondary)
                }

                SectionCard("App Visibility") {
                    Toggle("Menu Bar", isOn: Binding(
                        get: { visibility.showsMenuBar },
                        set: { visibility.setMenuBarVisible($0) }
                    ))
                    .disabled(visibility.showsMenuBar && !visibility.showsDockAndCommandTab)

                    Divider()

                    Toggle("Dock & Command-Tab", isOn: Binding(
                        get: { visibility.showsDockAndCommandTab },
                        set: { visibility.setDockAndCommandTabVisible($0) }
                    ))
                    .disabled(visibility.showsDockAndCommandTab && !visibility.showsMenuBar)

                    Text("macOS controls Dock and Command-Tab visibility together. At least one of Menu Bar or Dock must remain visible.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                SectionCard("Daemon") {
                    LabeledContent("Status", value: model.lifecycleState.title)
                    LabeledContent("Architecture", value: "App · CLI · MCP → daemon")
                    Button("Repair Installation") { Task { await model.repair() } }
                }

                SectionCard("Updates") {
                    LabeledContent("Installed version", value: Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "Development")
                    if let availableVersion = updates.availableVersion {
                        LabeledContent("Available version", value: availableVersion)
                    }
                    Button(updates.buttonTitle) { updates.checkForUpdates() }
                        .disabled(!updates.canCheckForUpdates)
                    Toggle("Check automatically", isOn: Binding(
                        get: { updates.automaticallyChecksForUpdates },
                        set: { updates.setAutomaticUpdateChecks($0) }
                    ))
                    Text("Updates are checked automatically and verified with Key Session's Ed25519 release key before installation.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                SectionCard("Privacy") {
                    Text("The UI receives consumer labels, profile names, lease timing, and audit metadata. Consumer capabilities, active secrets, and command output never enter the UI.")
                        .foregroundStyle(.secondary)
                }
            }
            .padding(30)
            .frame(maxWidth: 800, alignment: .leading)
        }
    }
}
