import Foundation

enum Fixtures {
    static let loginSuccess = """
    {"code":200,"message":"success","data":{"token":"secret-token","device_key":"device-key"}}
    """

    static let otpRequired = """
    {"code":402,"message":"Invalid 2FA code","data":null}
    """

    static let currentUser = """
    {"code":200,"message":"success","data":{"id":7,"username":"alice","ignored":true}}
    """

    static let directoryWithOptionalFieldsMissing = """
    {"code":200,"message":"success","data":{"content":[{"name":"movie.mp4","size":922337,"is_dir":false,"modified":"2026-08-19T10:11:12.123456789Z","created":"2026-08-19T10:00:00Z"}]}}
    """

    static func data(_ value: String) -> Data { Data(value.utf8) }
}
