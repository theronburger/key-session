import Foundation

public enum DaemonClientError: Error, CustomStringConvertible, Sendable {
    case unavailable(String)
    case invalidEndpoint
    case unauthorized
    case contract(String)
    case unexpectedStatus(Int)

    public var description: String {
        switch self {
        case .unavailable(let message): "Daemon unavailable: \(message)"
        case .invalidEndpoint: "The daemon endpoint descriptor is invalid."
        case .unauthorized: "The daemon rejected this app's credentials."
        case .contract(let message): message
        case .unexpectedStatus(let status): "The daemon returned HTTP \(status)."
        }
    }
}

public struct DaemonClient: Sendable {
    private let descriptor: RuntimeDescriptor
    private let session: URLSession
    private let decoder: JSONDecoder
    private let encoder: JSONEncoder

    public init(descriptor: RuntimeDescriptor) throws {
        guard descriptor.schemaVersion == keySessionContractVersion,
              let url = URL(string: descriptor.endpoint),
              url.scheme == "http", url.host == "127.0.0.1", url.port != nil,
              url.path.isEmpty, url.query == nil, url.fragment == nil,
              !descriptor.token.isEmpty else {
            throw DaemonClientError.invalidEndpoint
        }
        self.descriptor = descriptor
        let configuration = URLSessionConfiguration.ephemeral
        configuration.timeoutIntervalForRequest = 300
        configuration.requestCachePolicy = .reloadIgnoringLocalAndRemoteCacheData
        configuration.urlCache = nil
        configuration.httpCookieStorage = nil
        session = URLSession(configuration: configuration)
        decoder = Self.makeDecoder()
        encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .convertToSnakeCase
    }

    public static func connect(fileManager: FileManager = .default) throws -> DaemonClient {
        let descriptorURL = try RuntimeLocation.descriptorURL(fileManager: fileManager)
        let attributes = try fileManager.attributesOfItem(atPath: descriptorURL.path)
        guard let permissions = attributes[.posixPermissions] as? NSNumber,
              permissions.intValue & 0o077 == 0 else {
            throw DaemonClientError.invalidEndpoint
        }
        let data = try Data(contentsOf: descriptorURL, options: .mappedIfSafe)
        let descriptor = try Self.makeDecoder().decode(RuntimeDescriptor.self, from: data)
        return try DaemonClient(descriptor: descriptor)
    }

	public func snapshot() async throws -> StatusSnapshot {
		try await get("/v2/status", as: StatusSnapshot.self)
	}

    public func doctor() async throws -> DoctorReport {
		try await get("/v2/doctor", as: DoctorReport.self)
	}

	public func revoke(consumerId: String, leaseId: String? = nil) async throws {
		let _: EmptyResponse = try await send(
			"/v2/admin/revoke",
			method: "POST",
			body: AdminRevokeRequest(consumerId: consumerId, leaseId: leaseId),
			as: EmptyResponse.self
		)
	}

    public func saveProfile(_ request: ProfileRequest) async throws {
		let _: EmptyResponse = try await send("/v2/profiles", method: "POST", body: request, as: EmptyResponse.self)
    }

    public func beginProfileManagement(_ name: String) async throws -> ProfileManagementAuthorization {
        guard let escaped = name.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) else { throw DaemonClientError.invalidEndpoint }
		return try await send("/v2/profiles/\(escaped)/management", method: "POST", body: EmptyBody(), as: ProfileManagementAuthorization.self)
    }

    public func updateProfile(_ name: String, request: ProfileUpdateRequest) async throws {
        guard let escaped = name.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) else { throw DaemonClientError.invalidEndpoint }
		let _: EmptyResponse = try await send("/v2/profiles/\(escaped)", method: "PUT", body: request, as: EmptyResponse.self)
    }

    public func deleteProfile(_ name: String, managementToken: String) async throws {
        guard let escaped = name.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) else { throw DaemonClientError.invalidEndpoint }
        let _: EmptyResponse = try await send(
			"/v2/profiles/\(escaped)",
            method: "DELETE",
            body: ProfileManagementTokenRequest(managementToken: managementToken),
            as: EmptyResponse.self
        )
    }

    public func endProfileManagement(_ name: String, managementToken: String) async throws {
        guard let escaped = name.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) else { throw DaemonClientError.invalidEndpoint }
        let _: EmptyResponse = try await send(
			"/v2/profiles/\(escaped)/management/end",
            method: "POST",
            body: ProfileManagementTokenRequest(managementToken: managementToken),
            as: EmptyResponse.self
        )
    }

    private func get<Value: Decodable>(_ path: String, as type: Value.Type) async throws -> Value {
        try await send(path, method: "GET", body: Optional<EmptyBody>.none, as: type)
    }

    private func send<Body: Encodable, Value: Decodable>(_ path: String, method: String, body: Body?, as type: Value.Type) async throws -> Value {
        guard let url = URL(string: descriptor.endpoint + path) else { throw DaemonClientError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.cachePolicy = .reloadIgnoringLocalAndRemoteCacheData
        request.setValue("Bearer \(descriptor.token)", forHTTPHeaderField: "Authorization")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let body {
            request.httpBody = try encoder.encode(body)
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        let data: Data
        let response: URLResponse
        do { (data, response) = try await session.data(for: request) }
        catch { throw DaemonClientError.unavailable(error.localizedDescription) }
        guard let http = response as? HTTPURLResponse else { throw DaemonClientError.unavailable("Response was not HTTP") }
        guard http.value(forHTTPHeaderField: "Cache-Control")?.contains("no-store") == true else {
            throw DaemonClientError.contract("The daemon response omitted required security headers.")
        }
        switch http.statusCode {
        case 200..<300:
            return try decoder.decode(type, from: data)
        case 401, 403:
            throw DaemonClientError.unauthorized
        default:
            if let envelope = try? decoder.decode(ErrorEnvelope.self, from: data) {
                throw DaemonClientError.contract(envelope.error.message)
            }
            throw DaemonClientError.unexpectedStatus(http.statusCode)
        }
    }

    private static func makeDecoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        decoder.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let value = try container.decode(String.self)
            let fractional = ISO8601DateFormatter()
            fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            if let date = fractional.date(from: value) { return date }
            let standard = ISO8601DateFormatter()
            if let date = standard.date(from: value) { return date }
            throw DecodingError.dataCorruptedError(in: container, debugDescription: "Invalid RFC 3339 date")
        }
        return decoder
    }
}

public enum RuntimeLocation {
    public static func rootURL(fileManager: FileManager = .default) throws -> URL {
        let support = fileManager.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? fileManager.homeDirectoryForCurrentUser.appending(path: "Library/Application Support")
        return support.appending(path: "key-session")
    }

    public static func descriptorURL(fileManager: FileManager = .default) throws -> URL {
        try rootURL(fileManager: fileManager).appending(path: "runtime/endpoint.json")
    }
}

private struct EmptyBody: Encodable {}
private struct EmptyResponse: Decodable {}
