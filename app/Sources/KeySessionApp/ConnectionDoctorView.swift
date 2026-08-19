import KeySessionKit
import SwiftUI

struct ConnectionDoctorView: View {
    @Bindable var model: AppModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 22) {
                HStack(alignment: .top) {
                    VStack(alignment: .leading, spacing: 4) {
                        Text("Connection Doctor").font(.largeTitle.weight(.bold))
                        Text("Inspect and repair the daemon boundary without reading any secrets.")
                            .font(.title3).foregroundStyle(.secondary)
                    }
                    Spacer()
                    Button { Task { await model.runDoctor() } } label: { Label("Run Checks", systemImage: "stethoscope") }
                    Button { Task { await model.repair() } } label: { Label("Repair", systemImage: "wrench.and.screwdriver") }
                        .buttonStyle(.borderedProminent)
                }

                SectionCard("Connection") {
                    HStack {
                        StatusDot(color: model.lifecycleState == .connected ? .green : .red)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(model.lifecycleState.title).font(.headline)
                            Text(model.lifecycleState.detail ?? "App, CLI, and MCP are using the authenticated local daemon.")
                                .font(.caption).foregroundStyle(.secondary)
                        }
                        Spacer()
                        if model.isWorking { ProgressView().controlSize(.small) }
                    }
                }

                if let report = model.doctorReport {
                    SectionCard("Checks") {
                        ForEach(Array(report.checks.enumerated()), id: \.element.id) { index, check in
                            HStack(alignment: .top, spacing: 12) {
                                Image(systemName: symbol(check.status)).foregroundStyle(tint(check.status)).frame(width: 20)
                                VStack(alignment: .leading, spacing: 3) {
                                    Text(check.name).font(.body.weight(.medium))
                                    Text(check.detail).font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
                                }
                                Spacer()
                            }
                            .padding(.vertical, 5)
                            if index < report.checks.count - 1 { Divider() }
                        }
                    }
                } else {
                    EmptyPanel(title: "Ready to inspect", message: "Run checks for daemon, runtime permissions, configuration, and approval identity.", systemImage: "stethoscope")
                }
            }
            .padding(30).frame(maxWidth: 980, alignment: .leading)
        }
        .task { if model.doctorReport == nil { await model.runDoctor() } }
    }

    private func symbol(_ status: String) -> String {
        switch status { case "pass": "checkmark.circle.fill"; case "fail": "xmark.circle.fill"; default: "info.circle.fill" }
    }

    private func tint(_ status: String) -> Color {
        switch status { case "pass": .green; case "fail": .red; default: .secondary }
    }
}
