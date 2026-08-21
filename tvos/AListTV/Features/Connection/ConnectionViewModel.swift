import Combine
import Foundation

typealias AListClientFactory = @MainActor (URL, String) -> any AListAPI

@MainActor
final class ConnectionViewModel: ObservableObject {
    enum State: Equatable {
        case idle
        case submitting
        case otpRequired
        case connected
        case failed(message: String)
    }

    @Published private(set) var state: State = .idle
    @Published private(set) var canRetryRecovery = false
    private(set) var activeAPI: (any AListAPI)?
    private(set) var activeConnection: StoredConnection?

    private let preferences: ConnectionPreferences
    private let credentialStore: any CredentialStore
    private let clientFactory: AListClientFactory

    private var pendingServerURL: URL?
    private var pendingUsername: String?
    private var pendingPassword: String?
    private var pendingClientID: String?

    init(
        preferences: ConnectionPreferences,
        credentialStore: any CredentialStore,
        clientFactory: @escaping AListClientFactory
    ) {
        self.preferences = preferences
        self.credentialStore = credentialStore
        self.clientFactory = clientFactory
    }

    func submit(serverURL: String, username: String, password: String) async {
        canRetryRecovery = false
        state = .submitting
        do {
            let baseURL = try ServerURLValidator.validate(serverURL)
            let clientID = preferences.stableClientID()
            let api = clientFactory(baseURL, clientID)
            pendingServerURL = baseURL
            pendingUsername = username
            pendingPassword = password
            pendingClientID = clientID
            activeAPI = api
            let login = try await api.login(username: username, password: password, otpCode: nil)
            try commitLogin(login)
        } catch AListAPIError.otpRequired {
            state = .otpRequired
        } catch {
            clearPendingCredentials()
            state = .failed(message: Self.message(for: error))
        }
    }

    func submitOTP(_ otpCode: String) async {
        guard let api = activeAPI,
              let username = pendingUsername,
              let password = pendingPassword else {
            clearPendingCredentials()
            state = .idle
            return
        }
        state = .submitting
        do {
            let login = try await api.login(username: username, password: password, otpCode: otpCode)
            try commitLogin(login)
        } catch AListAPIError.otpRequired {
            state = .otpRequired
        } catch {
            clearPendingCredentials()
            state = .failed(message: Self.message(for: error))
        }
    }

    func cancelOTP() {
        clearPendingCredentials()
        activeAPI = nil
        state = .idle
    }

    func restore() async {
        canRetryRecovery = false
        guard let connection = preferences.loadConnection() else {
            state = .idle
            return
        }
        let token: String?
        do {
            token = try credentialStore.loadToken()
        } catch {
            canRetryRecovery = true
            state = .failed(message: Self.message(for: error))
            return
        }
        guard token?.isEmpty == false else {
            state = .idle
            return
        }

        state = .submitting
        let api = clientFactory(connection.baseURL, connection.clientID)
        activeAPI = api
        activeConnection = connection
        do {
            _ = try await api.currentUser()
            state = .connected
        } catch AListAPIError.unauthorized {
            do {
                try credentialStore.deleteToken()
                activeAPI = nil
                activeConnection = nil
                state = .idle
            } catch {
                canRetryRecovery = true
                state = .failed(message: Self.message(for: error))
            }
        } catch {
            canRetryRecovery = true
            state = .failed(message: Self.message(for: error))
        }
    }

    func clearSession() throws {
        canRetryRecovery = false
        try credentialStore.deleteToken()
        activeAPI = nil
        activeConnection = nil
        clearPendingCredentials()
        state = .idle
    }

    private func commitLogin(_ login: LoginData) throws {
        guard let baseURL = pendingServerURL,
              let username = pendingUsername,
              let clientID = pendingClientID else {
            throw AListAPIError.invalidResponse
        }
        try credentialStore.saveToken(login.token)
        preferences.saveConnection(baseURL: baseURL, username: username, clientID: clientID)
        activeConnection = StoredConnection(baseURL: baseURL, username: username, clientID: clientID)
        clearPendingCredentials()
        state = .connected
    }

    private func clearPendingCredentials() {
        pendingServerURL = nil
        pendingUsername = nil
        pendingPassword = nil
        pendingClientID = nil
    }

    private static func message(for error: Error) -> String {
        (error as? LocalizedError)?.errorDescription ?? error.localizedDescription
    }
}
