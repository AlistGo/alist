import Foundation
import XCTest
@testable import AListTV

@MainActor
final class ConnectionViewModelTests: XCTestCase {
    private var defaults: UserDefaults!
    private var preferences: ConnectionPreferences!

    override func setUp() {
        super.setUp()
        let suite = "ConnectionViewModelTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)!
        defaults.removePersistentDomain(forName: suite)
        preferences = ConnectionPreferences(defaults: defaults)
    }

    override func tearDown() {
        defaults = nil
        preferences = nil
        super.tearDown()
    }

    func testOTPChallengeRetriesSameCredentials() async {
        let api = ConnectionFakeAPI(loginResults: [
            .failure(.otpRequired),
            .success(LoginData(token: "token", deviceKey: nil))
        ])
        let store = TestCredentialStore()
        var factoryClientIDs: [String] = []
        let viewModel = ConnectionViewModel(preferences: preferences, credentialStore: store) { _, clientID in
            factoryClientIDs.append(clientID)
            return api
        }

        await viewModel.submit(serverURL: "https://alist.example/", username: "alice", password: "secret")
        XCTAssertEqual(viewModel.state, .otpRequired)
        await viewModel.submitOTP("123456")

        XCTAssertEqual(viewModel.state, .connected)
        XCTAssertEqual(store.token, "token")
        let calls = await api.loginCalls
        XCTAssertEqual(calls, [
            .init(username: "alice", password: "secret", otp: nil),
            .init(username: "alice", password: "secret", otp: "123456")
        ])
        XCTAssertEqual(factoryClientIDs.count, 1)
        XCTAssertEqual(preferences.loadConnection()?.clientID, factoryClientIDs[0])
    }

    func testTokenSaveFailureDoesNotConnect() async {
        let api = ConnectionFakeAPI(loginResults: [.success(LoginData(token: "token", deviceKey: nil))])
        let store = TestCredentialStore(saveError: TestStoreError.failed)
        let viewModel = ConnectionViewModel(preferences: preferences, credentialStore: store) { _, _ in api }

        await viewModel.submit(serverURL: "https://alist.example", username: "alice", password: "secret")

        guard case .failed = viewModel.state else {
            return XCTFail("Expected persistence failure")
        }
        XCTAssertNil(preferences.loadConnection())
    }

    func testRecoveryUnauthorizedDeletesToken() async {
        let api = ConnectionFakeAPI(currentUserError: .unauthorized)
        let store = TestCredentialStore(token: "expired")
        let clientID = UUID().uuidString
        preferences.saveConnection(baseURL: URL(string: "https://alist.example")!, username: "alice", clientID: clientID)
        let viewModel = ConnectionViewModel(preferences: preferences, credentialStore: store) { _, _ in api }

        await viewModel.restore()

        XCTAssertEqual(viewModel.state, .idle)
        XCTAssertNil(store.token)
        XCTAssertEqual(store.deleteCount, 1)
    }

    func testRecoveryTransportFailureRetainsToken() async {
        let api = ConnectionFakeAPI(currentUserError: .transport(message: "offline"))
        let store = TestCredentialStore(token: "valid-token")
        preferences.saveConnection(
            baseURL: URL(string: "https://alist.example")!,
            username: "alice",
            clientID: UUID().uuidString
        )
        let viewModel = ConnectionViewModel(preferences: preferences, credentialStore: store) { _, _ in api }

        await viewModel.restore()

        XCTAssertEqual(viewModel.state, .failed(message: "offline"))
        XCTAssertEqual(store.token, "valid-token")
        XCTAssertEqual(store.deleteCount, 0)
    }

    func testSessionClearFailureRetainsConnectedSession() async {
        let api = ConnectionFakeAPI(loginResults: [.success(LoginData(token: "token", deviceKey: nil))])
        let store = TestCredentialStore(deleteError: TestStoreError.failed)
        let viewModel = ConnectionViewModel(preferences: preferences, credentialStore: store) { _, _ in api }

        await viewModel.submit(serverURL: "https://alist.example", username: "alice", password: "secret")
        XCTAssertThrowsError(try viewModel.clearSession())
        XCTAssertEqual(viewModel.state, .connected)
        XCTAssertEqual(store.token, "token")
        XCTAssertEqual(store.deleteCount, 0)
    }
}

private struct LoginCall: Equatable, Sendable {
    let username: String
    let password: String
    let otp: String?
}

private actor ConnectionFakeAPI: AListAPI {
    private var loginResults: [Result<LoginData, AListAPIError>]
    private let currentUserError: AListAPIError?
    private(set) var loginCalls: [LoginCall] = []

    init(
        loginResults: [Result<LoginData, AListAPIError>] = [],
        currentUserError: AListAPIError? = nil
    ) {
        self.loginResults = loginResults
        self.currentUserError = currentUserError
    }

    func login(username: String, password: String, otpCode: String?) async throws -> LoginData {
        loginCalls.append(LoginCall(username: username, password: password, otp: otpCode))
        guard !loginResults.isEmpty else { throw AListAPIError.invalidResponse }
        return try loginResults.removeFirst().get()
    }

    func currentUser() async throws -> CurrentUser {
        if let currentUserError { throw currentUserError }
        return CurrentUser(id: 1, username: "alice")
    }

    func list(path: String, page: Int, perPage: Int) async throws -> DirectoryPage {
        throw AListAPIError.invalidResponse
    }

    func get(path: String) async throws -> FileDetail {
        throw AListAPIError.invalidResponse
    }
}

private enum TestStoreError: Error, LocalizedError {
    case failed
    var errorDescription: String? { "store failed" }
}

private final class TestCredentialStore: CredentialStore, @unchecked Sendable {
    var token: String?
    var deleteCount = 0
    private let saveError: Error?
    private let deleteError: Error?

    init(token: String? = nil, saveError: Error? = nil, deleteError: Error? = nil) {
        self.token = token
        self.saveError = saveError
        self.deleteError = deleteError
    }

    func loadToken() throws -> String? { token }

    func saveToken(_ token: String) throws {
        if let saveError { throw saveError }
        self.token = token
    }

    func deleteToken() throws {
        if let deleteError { throw deleteError }
        token = nil
        deleteCount += 1
    }
}
