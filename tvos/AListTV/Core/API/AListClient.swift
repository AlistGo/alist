import Foundation

actor AListClient: AListAPI {
    private let baseURL: URL
    private let clientID: String
    private let tokenProvider: @Sendable () -> String?
    private let session: URLSession
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()

    init(
        baseURL: URL,
        clientID: String,
        tokenProvider: @escaping @Sendable () -> String?,
        session: URLSession = .shared
    ) {
        self.baseURL = baseURL
        self.clientID = clientID
        self.tokenProvider = tokenProvider
        self.session = session
    }

    func login(username: String, password: String, otpCode: String?) async throws -> LoginData {
        try await send(
            path: "/api/auth/login",
            method: "POST",
            body: LoginRequest(username: username, password: password, otpCode: otpCode),
            authenticated: false
        )
    }

    func currentUser() async throws -> CurrentUser {
        try await send(path: "/api/me", method: "GET", bodyData: nil, authenticated: true)
    }

    func list(path: String, page: Int, perPage: Int) async throws -> DirectoryPage {
        try await send(
            path: "/api/fs/list",
            method: "POST",
            body: ListRequest(path: path, page: page, perPage: perPage, refresh: false),
            authenticated: true
        )
    }

    func get(path: String) async throws -> FileDetail {
        try await send(
            path: "/api/fs/get",
            method: "POST",
            body: GetRequest(path: path),
            authenticated: true
        )
    }

    private func send<Response: Decodable, Body: Encodable>(
        path: String,
        method: String,
        body: Body,
        authenticated: Bool
    ) async throws -> Response {
        let bodyData: Data
        do {
            bodyData = try encoder.encode(body)
        } catch {
            throw AListAPIError.invalidResponse
        }
        return try await send(path: path, method: method, bodyData: bodyData, authenticated: authenticated)
    }

    private func send<Response: Decodable>(
        path: String,
        method: String,
        bodyData: Data?,
        authenticated: Bool
    ) async throws -> Response {
        let request = try makeRequest(
            path: path,
            method: method,
            bodyData: bodyData,
            authenticated: authenticated
        )

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch let error as AListAPIError {
            throw error
        } catch {
            throw AListAPIError.transport(message: error.localizedDescription)
        }

        guard let http = response as? HTTPURLResponse else {
            throw AListAPIError.invalidResponse
        }
        if let mapped = Self.mappedError(code: http.statusCode, message: "") {
            throw mapped
        }

        let envelope: APIEnvelope<Response>
        do {
            envelope = try decoder.decode(APIEnvelope<Response>.self, from: data)
        } catch {
            throw AListAPIError.invalidResponse
        }

        if let mapped = Self.mappedError(code: envelope.code, message: envelope.message) {
            throw mapped
        }
        guard (200 ... 299).contains(http.statusCode), envelope.code == 200,
              let payload = envelope.data else {
            let code = envelope.code == 200 ? http.statusCode : envelope.code
            throw AListAPIError.server(code: code, message: envelope.message)
        }
        return payload
    }

    private func makeRequest(
        path: String,
        method: String,
        bodyData: Data?,
        authenticated: Bool
    ) throws -> URLRequest {
        guard var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: false),
              components.scheme?.lowercased() == "https",
              components.host?.isEmpty == false else {
            throw AListAPIError.invalidServerURL
        }
        let prefix = components.path == "/" ? "" : components.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        components.path = "/" + ([prefix, path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))]
            .filter { !$0.isEmpty }
            .joined(separator: "/"))
        components.query = nil
        components.fragment = nil
        guard let url = components.url else {
            throw AListAPIError.invalidServerURL
        }

        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue(clientID, forHTTPHeaderField: "Client-Id")
        if let bodyData {
            request.httpBody = bodyData
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        if authenticated, let token = tokenProvider(), !token.isEmpty {
            request.setValue(token, forHTTPHeaderField: "Authorization")
        }
        return request
    }

    private static func mappedError(code: Int, message: String) -> AListAPIError? {
        switch code {
        case 200 ... 299:
            return nil
        case 401:
            return .unauthorized
        case 402:
            return .otpRequired
        case 429:
            return .rateLimited
        default:
            return message.isEmpty ? nil : .server(code: code, message: message)
        }
    }
}
