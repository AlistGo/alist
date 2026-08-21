import XCTest
@testable import AListTV

final class AListFileTypeTests: XCTestCase {
    func testAListMediaTypeMapping() {
        XCTAssertEqual(AListObject(virtualPath: "/video.mp4", name: "video.mp4", isDirectory: false, type: 2).fileType, .video)
        XCTAssertEqual(AListObject(virtualPath: "/audio.m4a", name: "audio.m4a", isDirectory: false, type: 3).fileType, .audio)
        XCTAssertEqual(AListObject(virtualPath: "/cover.jpg", name: "cover.jpg", isDirectory: false, type: 5).fileType, .image)
    }
}
