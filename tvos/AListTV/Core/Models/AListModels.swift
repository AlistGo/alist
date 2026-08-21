import Foundation

struct APIEnvelope<Payload: Decodable>: Decodable {
    let code: Int
    let message: String
    let data: Payload?
}

struct LoginRequest: Encodable, Equatable {
    let username: String
    let password: String
    let otpCode: String?

    enum CodingKeys: String, CodingKey {
        case username, password
        case otpCode = "otp_code"
    }
}

struct LoginData: Decodable, Equatable, Sendable {
    let token: String
    let deviceKey: String?

    enum CodingKeys: String, CodingKey {
        case token
        case deviceKey = "device_key"
    }
}

struct CurrentUser: Decodable, Equatable, Sendable {
    let id: UInt64
    let username: String
}

struct ListRequest: Encodable, Equatable {
    let path: String
    let page: Int
    let perPage: Int
    let refresh: Bool

    enum CodingKeys: String, CodingKey {
        case path, page, refresh
        case perPage = "per_page"
    }
}

struct DirectoryPage: Decodable, Equatable, Sendable {
    let content: [AListObject]
    let hasMore: Bool?
    let page: Int?
    let perPage: Int?

    enum CodingKeys: String, CodingKey {
        case content, page
        case hasMore = "has_more"
        case perPage = "per_page"
    }
}

enum AListFileType: Int, Sendable {
    case unknown = 0
    case folder = 1
    case video = 2
    case audio = 3
    case text = 4
    case image = 5
}

struct AListObject: Codable, Equatable, Identifiable, Sendable {
    let virtualPath: String?
    let name: String
    let size: Int64
    let isDirectory: Bool
    let modified: String?
    let created: String?
    let type: Int?
    let thumbnail: String?

    var id: String { virtualPath ?? name }
    var fileType: AListFileType {
        AListFileType(rawValue: type ?? AListFileType.unknown.rawValue) ?? .unknown
    }


    enum CodingKeys: String, CodingKey {
        case name, size, modified, created, type
        case virtualPath = "virtual_path"
        case isDirectory = "is_dir"
        case thumbnail = "thumb"
    }

    init(
        virtualPath: String?,
        name: String,
        size: Int64 = 0,
        isDirectory: Bool,
        modified: String? = nil,
        created: String? = nil,
        type: Int? = nil,
        thumbnail: String? = nil
    ) {
        self.virtualPath = virtualPath
        self.name = name
        self.size = size
        self.isDirectory = isDirectory
        self.modified = modified
        self.created = created
        self.type = type
        self.thumbnail = thumbnail
    }

    func withVirtualPath(_ path: String) -> AListObject {
        AListObject(
            virtualPath: path,
            name: name,
            size: size,
            isDirectory: isDirectory,
            modified: modified,
            created: created,
            type: type,
            thumbnail: thumbnail
        )
    }
}

struct GetRequest: Encodable, Equatable {
    let path: String
}

struct FileDetail: Decodable, Equatable, Sendable {
    let rawURL: String
    let virtualPath: String?
    let name: String
    let size: Int64
    let isDirectory: Bool
    let modified: String?
    let created: String?
    let type: Int?
    let thumbnail: String?

    enum CodingKeys: String, CodingKey {
        case name, size, modified, created, type
        case rawURL = "raw_url"
        case virtualPath = "virtual_path"
        case isDirectory = "is_dir"
        case thumbnail = "thumb"
    }
}
