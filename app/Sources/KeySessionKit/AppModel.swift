import Foundation
import Observation

public enum SidebarSelection: Hashable, Sendable {
    case profiles
    case activity
    case doctor
    case settings
}

public struct ProfileManagementSession: Identifiable, Sendable {
    public var id: String { managementToken }
    public let profile: KeyProfile
    public let managementToken: String
    public let secret: String
    public let expiresAt: Date
}

@MainActor
@Observable
public final class AppModel {
    public private(set) var snapshot: StatusSnapshot?
    public private(set) var doctorReport: DoctorReport?
    public private(set) var agentConnectionsReport: AgentConnectionsReport?
    public private(set) var agentConnectionsError: String?
    public private(set) var lifecycleState: DaemonLifecycleState = .connecting
    public private(set) var lastRefreshedAt: Date?
    public private(set) var isWorking = false
    public var selection: SidebarSelection = .profiles
    public var presentedProfileEditor: KeyProfile?
    public var showsNewProfile = false
    public var errorMessage: String?
    public private(set) var authorizingProfileName: String?
    public private(set) var repairingAgentHosts: Set<String> = []

    @ObservationIgnored private let lifecycle = DaemonLifecycleController()
    @ObservationIgnored private var client: DaemonClient?
    @ObservationIgnored private var pollingTask: Task<Void, Never>?
    @ObservationIgnored private let preferences = UserDefaults.standard

    private static let suggestedAgentSetupKey = "hasSuggestedAgentSetup"

    public init() {}

	public var consumers: [KeyConsumer] { snapshot?.consumers ?? [] }
	public var activeLeases: [KeyLease] { consumers.flatMap(\.leases) }
    public var profiles: [KeyProfile] { snapshot?.profiles ?? [] }
    public var events: [AuditEvent] { snapshot?.events ?? [] }

    public func startPolling() {
        guard pollingTask == nil else { return }
        pollingTask = Task { [weak self] in
            guard let self else { return }
            await self.refresh()
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(3))
                await self.refresh(showErrors: false)
            }
        }
    }

    public func refresh(showErrors: Bool = true) async {
        do {
            let connected = try await lifecycle.connect()
            client = connected
            snapshot = try await connected.snapshot()
            lifecycleState = .connected
            lastRefreshedAt = Date()
            if agentConnectionsReport == nil {
                do {
                    agentConnectionsReport = try await connected.agentConnections()
                    agentConnectionsError = nil
                    if agentConnectionsReport?.connections.contains(where: \.canRepair) == true,
                       !preferences.bool(forKey: Self.suggestedAgentSetupKey) {
                        selection = .doctor
                        preferences.set(true, forKey: Self.suggestedAgentSetupKey)
                    }
                } catch {
                    agentConnectionsError = String(describing: error)
                }
            }
        } catch {
            lifecycleState = .unavailable(String(describing: error))
            if showErrors { errorMessage = String(describing: error) }
        }
    }

	public func revoke(consumerId: String, leaseId: String? = nil) async {
		_ = await perform {
			let client = try await self.requireClient()
			try await client.revoke(consumerId: consumerId, leaseId: leaseId)
		}
	}

    public func saveProfile(name: String, environmentVariable: String, durationSeconds: Int, secret: String) async -> Bool {
        await perform {
            let client = try await self.requireClient()
            try await client.saveProfile(ProfileRequest(name: name, environmentVariable: environmentVariable, defaultLeaseSeconds: durationSeconds, secret: secret))
        }
    }

    public func unlockProfileManagement(_ profile: KeyProfile) async -> ProfileManagementSession? {
        guard !isWorking else { return nil }
        isWorking = true
        authorizingProfileName = profile.name
        defer {
            authorizingProfileName = nil
            isWorking = false
        }
        do {
            let client = try await requireClient()
            let authorization = try await client.beginProfileManagement(profile.name)
            return ProfileManagementSession(
                profile: profile,
                managementToken: authorization.managementToken,
                secret: authorization.secret,
                expiresAt: authorization.expiresAt
            )
        } catch {
            errorMessage = String(describing: error)
            return nil
        }
    }

    public func updateProfile(_ session: ProfileManagementSession, environmentVariable: String, durationSeconds: Int, secret: String) async -> Bool {
        await perform {
            let client = try await self.requireClient()
            try await client.updateProfile(
                session.profile.name,
                request: ProfileUpdateRequest(
                    environmentVariable: environmentVariable,
                    defaultLeaseSeconds: durationSeconds,
                    secret: secret,
                    managementToken: session.managementToken
                )
            )
        }
    }

    public func delete(_ session: ProfileManagementSession) async -> Bool {
        await perform {
            let client = try await self.requireClient()
            try await client.deleteProfile(session.profile.name, managementToken: session.managementToken)
        }
    }

    public func endProfileManagement(profileName: String, managementToken: String) async {
        guard let client = try? await requireClient() else { return }
        try? await client.endProfileManagement(profileName, managementToken: managementToken)
    }

    public func runDoctor() async {
        isWorking = true
        defer { isWorking = false }
        do {
            let client = try await requireClient()
            doctorReport = try await client.doctor()
            do {
                agentConnectionsReport = try await client.agentConnections()
                agentConnectionsError = nil
            } catch {
                agentConnectionsError = String(describing: error)
            }
            await refresh(showErrors: false)
        } catch { errorMessage = String(describing: error) }
    }

    public func repairAgentConnection(_ host: String) async {
        guard !repairingAgentHosts.contains(host) else { return }
        repairingAgentHosts.insert(host)
        defer { repairingAgentHosts.remove(host) }
        do {
            let client = try await requireClient()
            agentConnectionsReport = try await client.repairAgentConnections(host: host)
            agentConnectionsError = nil
        } catch {
            errorMessage = String(describing: error)
        }
    }

    public func repairAllAgentConnections() async {
        guard repairingAgentHosts.isEmpty else { return }
        repairingAgentHosts = Set(agentConnectionsReport?.connections.filter(\.canRepair).map(\.host) ?? [])
        defer { repairingAgentHosts.removeAll() }
        do {
            let client = try await requireClient()
            agentConnectionsReport = try await client.repairAgentConnections()
            agentConnectionsError = nil
        } catch {
            errorMessage = String(describing: error)
        }
    }

    public func repair() async {
        lifecycleState = .repairing
        isWorking = true
        defer { isWorking = false }
        do {
            client = try await lifecycle.repair()
            await refresh()
            await runDoctor()
        } catch {
            lifecycleState = .unavailable(String(describing: error))
            errorMessage = String(describing: error)
        }
    }

    private func perform(_ operation: () async throws -> Void) async -> Bool {
        guard !isWorking else { return false }
        isWorking = true
        defer { isWorking = false }
        do {
            try await operation()
            await refresh(showErrors: false)
            return true
        } catch {
            errorMessage = String(describing: error)
            return false
        }
    }

    private func requireClient() async throws -> DaemonClient {
        if let client { return client }
        let connected = try await lifecycle.connect()
        client = connected
        return connected
    }
}
