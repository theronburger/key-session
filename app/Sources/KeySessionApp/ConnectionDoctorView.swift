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

                SectionCard("Agent Connections") {
                    if let report = model.agentConnectionsReport {
                        ForEach(Array(report.connections.enumerated()), id: \.element.id) { index, connection in
                            HStack(alignment: .top, spacing: 12) {
                                Image(systemName: agentSymbol(connection.state))
                                    .foregroundStyle(agentTint(connection.state))
                                    .frame(width: 20)
                                VStack(alignment: .leading, spacing: 4) {
                                    Text(connection.displayName).font(.body.weight(.medium))
                                    HStack(spacing: 8) {
                                        connectionBadge("MCP", state: connection.mcpState)
                                        connectionBadge("Skill", state: connection.skillState)
                                    }
                                    Text(connection.detail)
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                        .fixedSize(horizontal: false, vertical: true)
                                }
                                Spacer()
                                if connection.canRepair {
                                    Button(model.repairingAgentHosts.contains(connection.host) ? "Connecting…" : "Connect") {
                                        Task { await model.repairAgentConnection(connection.host) }
                                    }
                                    .controlSize(.small)
                                    .disabled(model.repairingAgentHosts.contains(connection.host))
                                }
                            }
                            .padding(.vertical, 5)
                            if index < report.connections.count - 1 { Divider() }
                        }
                        if report.connections.contains(where: \.canRepair) {
                            Divider()
                            HStack {
                                Text("Key Session uses each agent's own CLI and never stores a shared credential in MCP configuration.")
                                    .font(.caption).foregroundStyle(.secondary)
                                Spacer()
                                Button("Connect Detected Agents") {
                                    Task { await model.repairAllAgentConnections() }
                                }
                                .buttonStyle(.borderedProminent)
                                .disabled(!model.repairingAgentHosts.isEmpty)
                            }
                        }
                    } else if let error = model.agentConnectionsError {
                        Label(error, systemImage: "exclamationmark.triangle.fill")
                            .font(.caption)
                            .foregroundStyle(.red)
                            .textSelection(.enabled)
                    } else {
                        ProgressView("Looking for Codex and Claude Code…")
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

    private func agentSymbol(_ state: String) -> String {
        switch state {
        case "connected": "checkmark.circle.fill"
        case "missing", "needs_repair": "wrench.and.screwdriver.fill"
        case "refused": "hand.raised.fill"
        default: "minus.circle"
        }
    }

    private func agentTint(_ state: String) -> Color {
        switch state {
        case "connected": .green
        case "missing", "needs_repair": .orange
        case "refused": .red
        default: .secondary
        }
    }

    private func connectionBadge(_ label: String, state: String) -> some View {
        Text("\(label) · \(badgeLabel(state))")
            .font(.caption2.weight(.semibold))
            .foregroundStyle(agentTint(state))
            .padding(.horizontal, 7)
            .padding(.vertical, 3)
            .background(agentTint(state).opacity(0.1), in: Capsule())
    }

    private func badgeLabel(_ state: String) -> String {
        switch state {
        case "connected": "Ready"
        case "missing": "Missing"
        case "needs_repair": "Update"
        case "refused": "Protected"
        default: "Not found"
        }
    }
}
