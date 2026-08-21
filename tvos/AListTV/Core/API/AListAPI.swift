import Foundation

protocol AListAPI: Sendable {
    func login(username: String, password: String, otpCode: String?) async throws -> LoginData
    func currentUser() async throws -> CurrentUser
    func list(path: String, page: Int, perPage: Int) async throws -> DirectoryPage
    func get(path: String) async throws -> FileDetail
}

enum AListAPIError: Error, Equatable, LocalizedError, Sendable {
    case invalidServerURL
    case insecureURL
    case transport(message: String)
    case invalidResponse
    case server(code: Int, message: String)
    case otpRequired
    case unauthorized
    case rateLimited
    case invalidRawURL

    var errorDescription: String? {
        switch self {
        case .invalidServerURL:
            return "Enter a valid absolute server URL."
        case .insecureURL:
            return "Only secure HTTPS connections are allowed."
        case let .transport(message):
            return message
        case .invalidResponse:
            return "The server returned an invalid response."
        case let .server(_, message):
            return message.isEmpty ? "The server rejected the request." : message
        case .otpRequired:
            return "A two-factor authentication code is required."
        case .unauthorized:
            return "The session is no longer authorized."
        case .rateLimited:
            return "Too many requests. Try again later."
        case .invalidRawURL:
            return "This item does not have a secure playable URL."
        }
    }
}
