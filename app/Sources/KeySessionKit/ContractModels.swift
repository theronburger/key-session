import Foundation

public let keySessionContractVersion = 2

public struct RuntimeDescriptor: Decodable, Sendable {
    public let schemaVersion: Int
    public let endpoint: String
    public let token: String
    public let daemonInstanceId: String
    public let daemonVersion: String
    public let pid: Int
    public let generatedAt: Date
}

public struct DaemonInfo: Decodable, Sendable {
    public let instanceId: String
    public let version: String
    public let startedAt: Date
}

public struct KeyProfile: Decodable, Identifiable, Hashable, Sendable {
    public var id: String { name }
    public let name: String
    public let environmentVariable: String
    public let defaultLeaseSeconds: Int
}

public struct KeyLease: Decodable, Identifiable, Sendable, Equatable {
	public let id: String
	public let consumerId: String
	public let consumerLabel: String
	public let profile: String
	public let environmentVariable: String
	public let reason: String
	public let grantedAt: Date
	public let expiresAt: Date
}

public struct KeyConsumer: Decodable, Identifiable, Sendable, Equatable {
	public let id: String
	public let label: String
	public let createdAt: Date
	public let expiresAt: Date
	public let leases: [KeyLease]
}

public struct AuditEvent: Decodable, Identifiable, Sendable {
    public let id: String
    public let kind: String
    public let profile: String?
	public let consumerId: String?
	public let consumerLabel: String?
    public let reason: String?
    public let detail: String?
    public let occurredAt: Date
}

public struct StatusSnapshot: Decodable, Sendable {
    public let schemaVersion: Int
    public let revision: Int
    public let generatedAt: Date
    public let daemon: DaemonInfo
    public let profiles: [KeyProfile]
	public let consumers: [KeyConsumer]
    public let events: [AuditEvent]
}

public struct DoctorCheck: Decodable, Identifiable, Sendable {
    public var id: String { name }
    public let name: String
    public let status: String
    public let detail: String
}

public struct DoctorReport: Decodable, Sendable {
    public let healthy: Bool
    public let checks: [DoctorCheck]
}

public struct AgentConnection: Decodable, Identifiable, Sendable, Equatable {
    public var id: String { host }
    public let host: String
    public let displayName: String
    public let state: String
    public let mcpState: String
    public let skillState: String
    public let detail: String
    public let canRepair: Bool
}

public struct AgentConnectionsReport: Decodable, Sendable, Equatable {
    public let connections: [AgentConnection]
}

public struct AgentConnectionRepairRequest: Encodable, Sendable {
    public let host: String

    public init(host: String = "") {
        self.host = host
    }
}

public struct AdminRevokeRequest: Encodable, Sendable {
	public let consumerId: String
	public let leaseId: String?

	public init(consumerId: String, leaseId: String? = nil) {
		self.consumerId = consumerId
		self.leaseId = leaseId
	}
}

public struct ProfileRequest: Encodable, Sendable {
    public let name: String
    public let environmentVariable: String
    public let defaultLeaseSeconds: Int
    public let secret: String

    public init(name: String, environmentVariable: String, defaultLeaseSeconds: Int, secret: String) {
        self.name = name
        self.environmentVariable = environmentVariable
        self.defaultLeaseSeconds = defaultLeaseSeconds
        self.secret = secret
    }
}

public struct ProfileUpdateRequest: Encodable, Sendable {
    public let environmentVariable: String
    public let defaultLeaseSeconds: Int
    public let secret: String
    public let managementToken: String

    public init(environmentVariable: String, defaultLeaseSeconds: Int, secret: String, managementToken: String) {
        self.environmentVariable = environmentVariable
        self.defaultLeaseSeconds = defaultLeaseSeconds
        self.secret = secret
        self.managementToken = managementToken
    }
}

public struct ProfileManagementAuthorization: Decodable, Sendable {
    public let managementToken: String
    public let secret: String
    public let expiresAt: Date
}

public struct ProfileManagementTokenRequest: Encodable, Sendable {
    public let managementToken: String

    public init(managementToken: String) {
        self.managementToken = managementToken
    }
}

struct ErrorEnvelope: Decodable {
    let error: ContractError
}

struct ContractError: Decodable {
    let code: String
    let message: String
}

public enum KeySessionFormat {
    public static func duration(_ seconds: Int) -> String {
        let value = max(0, seconds)
        let hours = value / 3600
        let minutes = (value % 3600) / 60
        if hours > 0 { return minutes > 0 ? "\(hours)h \(minutes)m" : "\(hours)h" }
        return "\(max(1, minutes))m"
    }

    public static func remaining(until date: Date, now: Date = Date()) -> String {
        let seconds = max(0, Int(date.timeIntervalSince(now)))
        if seconds < 60 { return "\(seconds)s" }
        return duration(seconds)
    }

    public static func relative(_ date: Date, now: Date = Date()) -> String {
        let formatter = RelativeDateTimeFormatter()
        formatter.unitsStyle = .abbreviated
        return formatter.localizedString(for: date, relativeTo: now)
    }
}
