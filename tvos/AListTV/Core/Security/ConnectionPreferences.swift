import Foundation

struct StoredConnection: Equatable, Sendable {
    let baseURL: URL
    let username: String
    let clientID: String
}

struct ConnectionPreferences {
    private enum Key {
        static let baseURL = "com.alist.tv.base-url"
        static let username = "com.alist.tv.username"
        static let clientID = "com.alist.tv.client-id"
    }

    private let defaults: UserDefaults

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    func stableClientID() -> String {
        if let existing = defaults.string(forKey: Key.clientID), UUID(uuidString: existing) != nil {
            return existing
        }
        let generated = UUID().uuidString
        defaults.set(generated, forKey: Key.clientID)
        return generated
    }

    func saveConnection(baseURL: URL, username: String, clientID: String) {
        defaults.set(baseURL.absoluteString, forKey: Key.baseURL)
        defaults.set(username, forKey: Key.username)
        defaults.set(clientID, forKey: Key.clientID)
    }

    func loadConnection() -> StoredConnection? {
        guard let value = defaults.string(forKey: Key.baseURL),
              let baseURL = URL(string: value),
              let username = defaults.string(forKey: Key.username),
              let clientID = defaults.string(forKey: Key.clientID),
              UUID(uuidString: clientID) != nil else {
            return nil
        }
        return StoredConnection(baseURL: baseURL, username: username, clientID: clientID)
    }
}

enum ServerURLValidator {
    static func validate(_ input: String) throws -> URL {
        let trimmed = input.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty,
              var components = URLComponents(string: trimmed),
              components.scheme != nil else {
            throw AListAPIError.invalidServerURL
        }
        guard components.scheme?.lowercased() == "https" else {
            throw AListAPIError.insecureURL
        }
        guard let host = components.host, !host.isEmpty,
              components.user == nil,
              components.password == nil,
              components.query == nil,
              components.fragment == nil else {
            throw AListAPIError.invalidServerURL
        }

        var path = components.path
        while path.count > 1, path.hasSuffix("/") {
            path.removeLast()
        }
        components.path = path == "/" ? "" : path
        guard let normalized = components.url, normalized.host != nil else {
            throw AListAPIError.invalidServerURL
        }
        return normalized
    }
}
