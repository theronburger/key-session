import KeySessionKit
import SwiftUI

struct SettingsView: View {
    @Bindable var model: AppModel

    var body: some View {
        Form {
            Section("Daemon") {
                LabeledContent("Status", value: model.lifecycleState.title)
                LabeledContent("Architecture", value: "App · CLI · MCP → daemon")
                Button("Repair Installation") { Task { await model.repair() } }
            }
            Section("Updates") {
                LabeledContent("Installed version", value: Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "Development")
                Link("Check GitHub Releases", destination: URL(string: "https://github.com/theronburger/key-session/releases/latest")!)
            }
            Section("Privacy") {
				Text("The UI receives consumer labels, profile names, lease timing, and audit metadata. Consumer capabilities, active secrets, and command output never enter the UI.")
                    .foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped).padding().frame(width: 520, height: 390)
    }
}
