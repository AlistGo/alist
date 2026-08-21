import Foundation
import Security

protocol CredentialStore: Sendable {
    func loadToken() throws -> String?
    func saveToken(_ token: String) throws
    func deleteToken() throws
}

enum CredentialStoreError: Error, Equatable, LocalizedError, Sendable {
    case invalidData
    case keychain(status: OSStatus)

    var errorDescription: String? {
        switch self {
        case .invalidData:
            return "The saved session data is invalid."
        case let .keychain(status):
            return "Keychain operation failed (\(status))."
        }
    }
}

struct KeychainCredentialStore: CredentialStore {
    static let defaultService = "com.alist.tv.session"
    static let defaultAccount = "alist-token"

    private let service: String
    private let account: String

    init(service: String = defaultService, account: String = defaultAccount) {
        self.service = service
        self.account = account
    }

    func loadToken() throws -> String? {
        var query = baseQuery
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne

        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        if status == errSecItemNotFound {
            return nil
        }
        guard status == errSecSuccess else {
            throw CredentialStoreError.keychain(status: status)
        }
        guard let data = result as? Data,
              let token = String(data: data, encoding: .utf8) else {
            throw CredentialStoreError.invalidData
        }
        return token
    }

    func saveToken(_ token: String) throws {
        guard let data = token.data(using: .utf8) else {
            throw CredentialStoreError.invalidData
        }
        let update = [kSecValueData as String: data]
        let updateStatus = SecItemUpdate(baseQuery as CFDictionary, update as CFDictionary)
        if updateStatus == errSecSuccess {
            return
        }
        guard updateStatus == errSecItemNotFound else {
            throw CredentialStoreError.keychain(status: updateStatus)
        }

        var add = baseQuery
        add[kSecValueData as String] = data
        add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        let addStatus = SecItemAdd(add as CFDictionary, nil)
        guard addStatus == errSecSuccess else {
            throw CredentialStoreError.keychain(status: addStatus)
        }
    }

    func deleteToken() throws {
        let status = SecItemDelete(baseQuery as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw CredentialStoreError.keychain(status: status)
        }
    }

    private var baseQuery: [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account
        ]
    }
}

final class SessionCredentialStore: CredentialStore, @unchecked Sendable {
    private let backing: any CredentialStore
    private let lock = NSLock()
    private var cachedToken: String?

    init(backing: any CredentialStore) {
        self.backing = backing
    }

    var currentToken: String? {
        lock.lock()
        defer { lock.unlock() }
        return cachedToken
    }

    func loadToken() throws -> String? {
        let token = try backing.loadToken()
        lock.lock()
        cachedToken = token
        lock.unlock()
        return token
    }

    func saveToken(_ token: String) throws {
        try backing.saveToken(token)
        lock.lock()
        cachedToken = token
        lock.unlock()
    }

    func deleteToken() throws {
        try backing.deleteToken()
        lock.lock()
        cachedToken = nil
        lock.unlock()
    }
}
