import Darwin
import Foundation

public enum DaemonLifecycleState: Equatable, Sendable {
    case connecting
    case connected
    case repairing
    case unavailable(String)

    public var title: String {
        switch self {
        case .connecting: "Connecting"
        case .connected: "Connected"
        case .repairing: "Repairing"
        case .unavailable: "Needs attention"
        }
    }

    public var detail: String? {
        if case .unavailable(let message) = self { return message }
        return nil
    }
}

public actor DaemonLifecycleController {
    public static let launchAgentLabel = "com.theronburger.key-session.daemon"
    private var preparedInstallation = false

    public init() {}

    public func connect(installIfNeeded: Bool = true) async throws -> DaemonClient {
        if installIfNeeded && !preparedInstallation {
            try installAndStart()
            preparedInstallation = true
        }
        if let client = try? DaemonClient.connect(), (try? await client.snapshot()) != nil { return client }
        for delay in [100, 200, 400, 800, 1200] {
            try? await Task.sleep(for: .milliseconds(delay))
            if let client = try? DaemonClient.connect(), (try? await client.snapshot()) != nil { return client }
        }
        throw DaemonClientError.unavailable("The helper did not publish a usable endpoint.")
    }

    public func repair() async throws -> DaemonClient {
        try installAndStart(forceRestart: true)
        preparedInstallation = true
        return try await connect(installIfNeeded: false)
    }

    private func installAndStart(forceRestart: Bool = false) throws {
        let fileManager = FileManager.default
        let root = try RuntimeLocation.rootURL(fileManager: fileManager)
        let logsDirectory = root.appending(path: "logs")
        try createPrivateDirectory(logsDirectory)

        let source = try bundledDaemon()
        let installedHelper = root.appending(path: "Key Session Helper.app")
        let installedBinary = installedHelper.appending(path: "Contents/MacOS/KeySessionDaemon")
        let sourceData = try Data(contentsOf: source.executable, options: .mappedIfSafe)
        let installedData = try? Data(contentsOf: installedBinary, options: .mappedIfSafe)
        let helperWasReplaced = sourceData != installedData
        if helperWasReplaced {
            let temporary = root.appending(path: ".helper-\(UUID().uuidString).app")
            defer { try? fileManager.removeItem(at: temporary) }
            if let helper = source.helperBundle {
                try fileManager.copyItem(at: helper, to: temporary)
            } else {
                let temporaryBinary = temporary.appending(path: "Contents/MacOS/KeySessionDaemon")
                try fileManager.createDirectory(at: temporaryBinary.deletingLastPathComponent(), withIntermediateDirectories: true)
                try sourceData.write(to: temporaryBinary, options: .withoutOverwriting)
                try fileManager.setAttributes([.posixPermissions: 0o700], ofItemAtPath: temporaryBinary.path)
            }
            try? fileManager.removeItem(at: installedHelper)
            if rename(temporary.path, installedHelper.path) != 0 {
                throw DaemonClientError.unavailable("Could not install the daemon helper.")
            }
        }

        let launchAgents = fileManager.homeDirectoryForCurrentUser.appending(path: "Library/LaunchAgents")
        try fileManager.createDirectory(at: launchAgents, withIntermediateDirectories: true)
        let propertyListURL = launchAgents.appending(path: "\(Self.launchAgentLabel).plist")
        let propertyList: [String: Any] = [
            "Label": Self.launchAgentLabel,
            "ProgramArguments": [installedBinary.path, "_daemon"],
            "RunAtLoad": true,
            "KeepAlive": true,
            "ProcessType": "Interactive",
            "StandardOutPath": logsDirectory.appending(path: "daemon.stdout.log").path,
            "StandardErrorPath": logsDirectory.appending(path: "daemon.stderr.log").path,
        ]
        let data = try PropertyListSerialization.data(fromPropertyList: propertyList, format: .xml, options: 0)
        if (try? Data(contentsOf: propertyListURL)) != data {
            try data.write(to: propertyListURL, options: .atomic)
            try fileManager.setAttributes([.posixPermissions: 0o600], ofItemAtPath: propertyListURL.path)
        }

        let target = "gui/\(getuid())/\(Self.launchAgentLabel)"
        if forceRestart { _ = run("/bin/launchctl", ["bootout", target]) }
        let printResult = run("/bin/launchctl", ["print", target])
        if printResult != 0 {
            let domain = "gui/\(getuid())"
            guard run("/bin/launchctl", ["bootstrap", domain, propertyListURL.path]) == 0 else {
                throw DaemonClientError.unavailable("macOS could not register the Key Session helper.")
            }
        } else if (forceRestart || helperWasReplaced) && run("/bin/launchctl", ["kickstart", "-k", target]) != 0 {
            throw DaemonClientError.unavailable("macOS could not start the Key Session helper.")
        }
    }

    private struct BundledDaemon {
        let executable: URL
        let helperBundle: URL?
    }

    private func bundledDaemon() throws -> BundledDaemon {
        if let helper = Bundle.main.resourceURL?.appending(path: "Key Session Helper.app") {
            let executable = helper.appending(path: "Contents/MacOS/KeySessionDaemon")
            if FileManager.default.isExecutableFile(atPath: executable.path) {
                return BundledDaemon(executable: executable, helperBundle: helper)
            }
        }
        throw DaemonClientError.unavailable("The app bundle does not contain KeySessionDaemon.")
    }

    private func createPrivateDirectory(_ url: URL) throws {
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: url.path)
    }

    @discardableResult
    private func run(_ executable: String, _ arguments: [String]) -> Int32 {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: executable)
        process.arguments = arguments
        process.standardOutput = FileHandle.nullDevice
        process.standardError = FileHandle.nullDevice
        do { try process.run(); process.waitUntilExit(); return process.terminationStatus }
        catch { return 1 }
    }
}
