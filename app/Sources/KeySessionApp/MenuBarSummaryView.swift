import AppKit
import KeySessionKit
import SwiftUI

struct MenuBarSummaryView: View {
    @Bindable var model: AppModel
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            header
            Divider().padding(.vertical, 11)
            content
            Divider().padding(.vertical, 11)
            actions
        }
        .padding(14).frame(width: 420)
        .task { model.startPolling() }
    }

    private var header: some View {
        HStack(spacing: 11) {
            BrandIcon(size: 38)
            VStack(alignment: .leading, spacing: 2) {
                Text("Key Session").font(.headline)
                Text("\(model.lifecycleState.title) · access daemon").font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
            if let refreshed = model.lastRefreshedAt {
                Text("Updated \(KeySessionFormat.relative(refreshed))").font(.caption).foregroundStyle(.secondary)
            }
        }
    }

    @ViewBuilder private var content: some View {
		if let lease = model.activeLeases.first {
            TimelineView(.periodic(from: .now, by: 1)) { timeline in
                VStack(alignment: .leading, spacing: 12) {
                    HStack {
						Metric(value: lease.profile, label: model.activeLeases.count == 1 ? "Active profile" : "First of \(model.activeLeases.count) leases", tint: .green)
                        Metric(value: KeySessionFormat.remaining(until: lease.expiresAt, now: timeline.date), label: "Remaining")
                    }
                    VStack(alignment: .leading, spacing: 4) {
						Text(lease.consumerLabel).font(.body.weight(.semibold))
                        Text(lease.reason).font(.caption).foregroundStyle(.secondary).lineLimit(3)
                    }
                    .padding(12).frame(maxWidth: .infinity, alignment: .leading)
					.background(.green.opacity(0.09), in: RoundedRectangle(cornerRadius: 10))
					Button("Revoke This Lease", role: .destructive) {
						Task { await model.revoke(consumerId: lease.consumerId, leaseId: lease.id) }
					}
                        .frame(maxWidth: .infinity).disabled(model.isWorking)
                }
            }
		} else if let consumer = model.consumers.first {
			TimelineView(.periodic(from: .now, by: 1)) { timeline in
				VStack(alignment: .leading, spacing: 10) {
					HStack(spacing: 11) {
						Image(systemName: "person.crop.circle.badge.checkmark").font(.title2).foregroundStyle(.green)
						VStack(alignment: .leading, spacing: 2) {
							Text(consumer.label).font(.headline)
							Text("No active leases · session expires in \(KeySessionFormat.remaining(until: consumer.expiresAt, now: timeline.date))")
								.font(.caption).foregroundStyle(.secondary)
						}
					}
					Button("End Consumer Session", role: .destructive) {
						Task { await model.revoke(consumerId: consumer.id) }
					}.frame(maxWidth: .infinity).disabled(model.isWorking)
				}
			}
		} else {
            VStack(alignment: .leading, spacing: 10) {
                HStack(spacing: 11) {
                    Image(systemName: "lock.fill").font(.title2).foregroundStyle(.secondary)
                    VStack(alignment: .leading, spacing: 2) {
                        Text("Access is locked").font(.headline)
						Text("No consumer currently holds a credential lease.").font(.caption).foregroundStyle(.secondary)
                    }
                }
                Divider()
                Label("Access requests originate from the CLI or MCP and appear here when active.", systemImage: "terminal")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var actions: some View {
        VStack(spacing: 9) {
            Button { openMainWindow() } label: {
                Label("Open Key Session", systemImage: "macwindow").frame(maxWidth: .infinity)
            }.buttonStyle(.bordered).controlSize(.large)
            HStack(spacing: 8) {
                Button { model.selection = .doctor; openMainWindow() } label: { Label("Doctor", systemImage: "stethoscope") }.frame(maxWidth: .infinity)
                Button { Task { await model.refresh() } } label: { Label("Refresh", systemImage: "arrow.clockwise") }.frame(maxWidth: .infinity)
                Button { model.selection = .settings; openMainWindow() } label: { Label("Settings", systemImage: "gearshape") }.frame(maxWidth: .infinity)
                Button { NSApplication.shared.terminate(nil) } label: { Label("Quit", systemImage: "power") }.frame(maxWidth: .infinity)
            }
        }
    }

    private func openMainWindow() {
        openWindow(id: "command-center")
        NSApplication.shared.activate(ignoringOtherApps: true)
    }
}
