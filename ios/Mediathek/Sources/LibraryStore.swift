import CryptoKit
import Foundation
import Network
import Security
import SwiftUI

struct Connection: Codable {
    let server: URL
    let name: String
    let token: String
}

private struct LoginReply: Decodable {
    let name: String
    let token: String
}

private struct LoginRequest: Encodable {
    let action: String
    let name: String
    let password: String
    let invite: String
    let native = true
}

private struct SyncRequest: Encodable { let operations: [Change] }
private struct ServerError: Decodable { let error: String }

private enum SyncError: LocalizedError {
    case request(String)
    case needsLogin
    var errorDescription: String? {
        switch self {
        case .request(let message): message
        case .needsLogin: "Bitte melde dich erneut an. Deine Änderungen bleiben auf dem iPhone gespeichert."
        }
    }
}

private final class NoRedirects: NSObject, URLSessionTaskDelegate, @unchecked Sendable {
    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
        completionHandler(nil)
    }
}

@MainActor
final class LibraryStore: ObservableObject {
    @Published private(set) var connection: Connection?
    @Published private(set) var state = LibraryState()
    @Published private(set) var syncing = false
    @Published var message = ""
    @Published var needsLogin = false

    private let monitor = NWPathMonitor()
    private let session = URLSession(configuration: .ephemeral, delegate: NoRedirects(), delegateQueue: nil)
    private let folder = URL.applicationSupportDirectory.appending(path: "Mediathek", directoryHint: .isDirectory)
    private var again = false
    private var networkReachable = true

    init() {
        if let credentials = Self.readCredentials() {
            do { try load(credentials) }
            catch { message = error.localizedDescription }
        }
        monitor.pathUpdateHandler = { [weak self] path in
            let reachable = path.status == .satisfied
            Task { @MainActor [weak self] in
                guard let self else { return }
                networkReachable = reachable
                if reachable { await sync() }
            }
        }
        monitor.start(queue: .global(qos: .utility))
    }

    private func location(_ credentials: Connection) -> URL {
        let key = credentials.server.absoluteString + "\n" + credentials.name
        let hash = SHA256.hash(data: Data(key.utf8)).map { String(format: "%02x", $0) }.joined()
        return folder.appending(path: hash + ".json")
    }

    private func load(_ credentials: Connection) throws {
        let file = location(credentials)
        let saved: LibraryState
        if FileManager.default.fileExists(atPath: file.path) {
            saved = try JSONDecoder().decode(LibraryState.self, from: Data(contentsOf: file))
        } else {
            saved = LibraryState()
        }
        connection = credentials
        state = saved
        needsLogin = false
    }

    private func persist(_ updated: LibraryState) throws {
        guard let connection else { throw SyncError.needsLogin }
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        try JSONEncoder().encode(updated).write(to: location(connection), options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
        state = updated
    }

    private static let identity: [String: Any] = [
        kSecClass as String: kSecClassGenericPassword,
        kSecAttrService as String: "Mediathek Konto",
        kSecAttrAccount as String: "active",
    ]

    private static func readCredentials() -> Connection? {
        var query = identity
        query[kSecReturnData as String] = true
        var value: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &value) == errSecSuccess, let data = value as? Data else { return nil }
        return try? JSONDecoder().decode(Connection.self, from: data)
    }

    private static func saveCredentials(_ connection: Connection) throws {
        let data = try JSONEncoder().encode(connection)
        let change: [String: Any] = [kSecValueData as String: data]
        let status = SecItemUpdate(identity as CFDictionary, change as CFDictionary)
        if status == errSecSuccess { return }
        if status == errSecItemNotFound {
            var add = identity.merging(change) { _, new in new }
            add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
            if SecItemAdd(add as CFDictionary, nil) == errSecSuccess { return }
        }
        throw SyncError.request("Die Anmeldung konnte nicht im Schlüsselbund gespeichert werden.")
    }

    func signIn(server: String, name: String, password: String, invite: String, register: Bool) async {
        guard !syncing else { return }
        let value = server.trimmingCharacters(in: .whitespacesAndNewlines)
        guard var parts = URLComponents(string: value), parts.scheme == "https", parts.host != nil,
              parts.user == nil, parts.password == nil, parts.query == nil, parts.fragment == nil,
              parts.path.isEmpty || parts.path == "/" else {
            message = "Gib die HTTPS-Adresse deines Kontoservers ein."
            return
        }
        parts.path = ""
        guard let url = parts.url else { return }
        syncing = true
        defer { syncing = false }
        do {
            let response: LoginReply = try await post(url: url.appending(path: "api/account/session"), token: nil,
                                                      payload: LoginRequest(action: register ? "register" : "login",
                                                                            name: name, password: password, invite: invite))
            let next = Connection(server: url, name: response.name, token: response.token)
            if let previous = connection, !state.pending.isEmpty,
               previous.server != next.server || previous.name != next.name {
                throw SyncError.request("Melde dich zuerst wieder mit dem ursprünglichen Konto an, um die offenen Änderungen zu übertragen.")
            }
            let old = connection
            let oldState = state
            try load(next)
            do { try Self.saveCredentials(next) }
            catch { connection = old; state = oldState; throw error }
            message = ""
            Task { await sync() }
        } catch { message = error.localizedDescription }
    }

    func signOut() async {
        guard !syncing, state.pending.isEmpty else {
            message = "Übertrage zuerst deine offenen Änderungen."
            return
        }
        if let connection {
            var request = URLRequest(url: connection.server.appending(path: "api/account/logout"), timeoutInterval: 4)
            request.httpMethod = "POST"
            request.setValue("Bearer " + connection.token, forHTTPHeaderField: "Authorization")
            _ = try? await session.data(for: request)
        }
        SecItemDelete(Self.identity as CFDictionary)
        connection = nil
        state = LibraryState()
        needsLogin = false
        message = ""
    }

    func save(_ item: Media, original: Media?) throws {
        var updated = state
        var item = item
        item.title = item.title.trimmingCharacters(in: .whitespacesAndNewlines)
        if item.title.isEmpty || item.title.utf8.count > 600 || (item.notes?.utf8.count ?? 0) > 16000 {
            throw SyncError.request("Prüfe den Titel und die Länge deiner Notizen.")
        }
        if let original {
            let fields = item.fieldsChanged(from: original)
            guard !fields.isEmpty else { return }
            updated.pending.append(Change("edit", item: item, base: original, fields: fields))
        } else {
            updated.pending.append(Change("add", item: item))
        }
        try persist(updated)
        Task { await sync() }
    }

    func delete(_ item: Media) throws {
        var updated = state
        updated.pending.append(Change("delete", item: item, base: item))
        try persist(updated)
        Task { await sync() }
    }

    func resolve(_ conflict: MergeConflict, keepLocal: Bool) throws {
        guard !syncing, let original = state.pending.first(where: { $0.id == conflict.id }) else { return }
        let intended = state.visible.first { $0.id == original.itemId }
        var updated = state
        let discarded = Set(updated.pending.filter { $0.itemId == original.itemId }.map(\.id))
        updated.pending.removeAll { discarded.contains($0.id) }
        updated.conflicts.removeAll { discarded.contains($0.id) }
        if keepLocal {
            if let current = conflict.current {
                if let intended {
                    let fields = intended.fieldsChanged(from: current)
                    if !fields.isEmpty { updated.pending.append(Change("edit", item: intended, base: current, fields: fields)) }
                } else {
                    updated.pending.append(Change("delete", item: current, base: current))
                }
            } else if var intended {
                intended.id = "manual:" + UUID().uuidString.lowercased()
                intended.revision = nil
                updated.pending.append(Change("add", item: intended))
            }
        }
        try persist(updated)
        Task { await sync() }
    }

    func sync() async {
        guard let connection, !needsLogin, networkReachable else { return }
        if syncing { again = true; return }
        syncing = true
        defer {
            syncing = false
            if again {
                again = false
                Task { await sync() }
            }
        }
        do {
            repeat {
                let conflicted = Set(state.conflicts.map(\.id))
                let blocked = Set(state.pending.filter { conflicted.contains($0.id) }.map(\.itemId))
                let batch = Array(state.pending.filter { !blocked.contains($0.itemId) }.prefix(8))
                let response: SyncReply = try await post(url: connection.server.appending(path: "api/sync"),
                                                         token: connection.token, payload: SyncRequest(operations: batch))
                let accepted = Set(response.accepted)
                var updated = state
                updated.entries = response.entries ?? []
                updated.lists = response.lists ?? []
                updated.pending.removeAll { accepted.contains($0.id) }
                updated.conflicts.removeAll { accepted.contains($0.id) }
                updated.conflicts.append(contentsOf: response.conflicts)
                try persist(updated)
                guard batch.count == 8 else { break }
            } while !Task.isCancelled
            message = ""
        } catch SyncError.needsLogin {
            needsLogin = true
            message = SyncError.needsLogin.localizedDescription
        } catch {
            if !Task.isCancelled {
                message = "Abgleich wartet. Deine Änderungen bleiben gespeichert."
            }
        }
    }

    private func post<T: Decodable, Payload: Encodable>(url: URL, token: String?, payload: Payload) async throws -> T {
        var request = URLRequest(url: url, timeoutInterval: 20)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if let token { request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization") }
        request.httpBody = try JSONEncoder().encode(payload)
        let (data, response) = try await session.data(for: request)
        guard let status = (response as? HTTPURLResponse)?.statusCode else {
            throw SyncError.request("Der Kontoserver ist nicht erreichbar.")
        }
        if status == 401, token != nil { throw SyncError.needsLogin }
        guard status == 200 else {
            let error = try? JSONDecoder().decode(ServerError.self, from: data)
            throw SyncError.request(error?.error ?? "Der Server hat die Anfrage abgelehnt.")
        }
        return try JSONDecoder().decode(T.self, from: data)
    }
}
