import KeySessionKit
import SwiftUI

struct CommandCenterView: View {
    @Bindable var model: AppModel
    @Bindable var updates: AppUpdateController
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        NavigationSplitView {
            sidebar
                .navigationSplitViewColumnWidth(min: 205, ideal: 230, max: 270)
        } detail: {
            detail
        }
        .navigationTitle(title)
        .toolbar {
            if updates.availableVersion != nil {
                ToolbarItem(placement: .automatic) {
                    Button { updates.checkForUpdates() } label: {
                        Label(updates.buttonTitle, systemImage: "arrow.down.circle.fill")
                    }
                    .help(updates.buttonTitle)
                }
            }
            ToolbarItem(placement: .primaryAction) {
                Button { Task { await model.refresh() } } label: {
                    Label("Refresh", systemImage: "arrow.clockwise")
                }
                .keyboardShortcut("r", modifiers: .command)
            }
        }
        .frame(minWidth: 760, minHeight: 560)
        .sheet(item: $model.presentedProfileEditor) { profile in
            ManageProfileSheet(model: model, profile: profile)
        }
        .sheet(isPresented: $model.showsNewProfile) {
            NewProfileSheet(model: model)
        }
        .alert("Key Session couldn't complete that action", isPresented: Binding(
            get: { model.errorMessage != nil },
            set: { if !$0 { model.errorMessage = nil } }
        )) {
            Button("OK") { model.errorMessage = nil }
        } message: { Text(model.errorMessage ?? "Unknown error") }
        .onReceive(NotificationCenter.default.publisher(for: .openKeySession)) { _ in
            openWindow(id: "command-center")
            NSApplication.shared.activate(ignoringOtherApps: true)
        }
    }

    private var sidebar: some View {
        VStack(spacing: 0) {
            HStack(spacing: 10) {
                BrandIcon(size: 34)
                VStack(alignment: .leading, spacing: 1) {
                    Text("Key Session").font(.headline)
                    Text("access daemon").font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
            }
            .padding(.horizontal, 14).padding(.vertical, 12)
            Divider()

            ScrollView {
                VStack(alignment: .leading, spacing: 3) {
                    SidebarRow(title: "Profiles", systemImage: "key.horizontal", selection: .profiles, model: model)
                    SidebarRow(title: "Activity", systemImage: "clock.arrow.circlepath", selection: .activity, model: model)
                    Text("Setup")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(.tertiary)
                        .padding(.leading, 8)
                        .padding(.top, 14)
                        .padding(.bottom, 4)
                    SidebarRow(title: "Connection Doctor", systemImage: lifecycleSymbol, selection: .doctor, model: model)
                }
                .padding(.horizontal, 8)
                .padding(.top, 10)
            }

            Divider()
            HStack(spacing: 7) {
                Image(systemName: lifecycleSymbol).foregroundStyle(lifecycleTint)
                VStack(alignment: .leading, spacing: 1) {
                    Text(model.lifecycleState.title).font(.caption.weight(.medium))
                    if let refreshed = model.lastRefreshedAt {
                        Text("Updated \(KeySessionFormat.relative(refreshed))").font(.caption2).foregroundStyle(.secondary)
                    }
                }
                Spacer()
            }
            .padding(.horizontal, 14).padding(.vertical, 10)
            .background(.bar)
        }
        .background(Color(nsColor: .windowBackgroundColor))
    }

    @ViewBuilder private var detail: some View {
        switch model.selection {
        case .profiles: OverviewView(model: model)
        case .activity: ActivityView(model: model)
        case .doctor: ConnectionDoctorView(model: model)
        }
    }

    private var title: String {
        switch model.selection {
        case .profiles: "Profiles"
        case .activity: "Activity"
        case .doctor: "Connection Doctor"
        }
    }

    private var lifecycleSymbol: String {
        switch model.lifecycleState {
        case .connected: "checkmark.circle.fill"
        case .connecting, .repairing: "arrow.trianglehead.2.clockwise.rotate.90"
        case .unavailable: "exclamationmark.triangle.fill"
        }
    }

    private var lifecycleTint: Color {
        switch model.lifecycleState {
        case .connected: .green
        case .connecting, .repairing: .secondary
        case .unavailable: .red
        }
    }
}

private struct SidebarRow: View {
    let title: String
    let systemImage: String
    let selection: SidebarSelection
    @Bindable var model: AppModel
    @State private var isHovering = false

    private var isSelected: Bool { model.selection == selection }

    var body: some View {
        Button { model.selection = selection } label: {
            Label(title, systemImage: systemImage)
                .font(.body.weight(isSelected ? .semibold : .regular))
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 10)
                .padding(.vertical, 8)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .background(
            Color.primary.opacity(isSelected ? 0.11 : (isHovering ? 0.055 : 0)),
            in: RoundedRectangle(cornerRadius: 8)
        )
        .onHover { isHovering = $0 }
    }
}
