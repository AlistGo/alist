import AVFoundation
import UIKit

actor ArtworkLoadLimiter {
    static let shared = ArtworkLoadLimiter(limit: 3)

    private var permits: Int
    private var waiters: [CheckedContinuation<Void, Never>] = []

    init(limit: Int) {
        permits = limit
    }

    func acquire() async {
        if permits > 0 {
            permits -= 1
            return
        }
        await withCheckedContinuation { waiters.append($0) }
    }

    func release() {
        if let waiter = waiters.first {
            waiters.removeFirst()
            waiter.resume()
        } else {
            permits += 1
        }
    }
}

enum MediaArtworkLoader {
    static func load(from url: URL, type: AListFileType) async -> UIImage? {
        let asset = AVURLAsset(url: url)

        if let artwork = await embeddedArtwork(from: asset) {
            return cardSized(artwork)
        }
        guard type == .video else { return nil }

        let generator = AVAssetImageGenerator(asset: asset)
        generator.appliesPreferredTrackTransform = true
        generator.maximumSize = CGSize(width: 640, height: 360)
        do {
            let image = try generator.copyCGImage(at: CMTime(seconds: 1, preferredTimescale: 600), actualTime: nil)
            return cardSized(UIImage(cgImage: image))
        } catch {
            return nil
        }
    }

    private static func embeddedArtwork(from asset: AVAsset) async -> UIImage? {
        guard let artwork = try? await asset.load(.commonMetadata).first(where: {
            $0.commonKey == .commonKeyArtwork
        }), let data = try? await artwork.load(.dataValue) else {
            return nil
        }
        return UIImage(data: data)
    }

    private static func cardSized(_ image: UIImage) -> UIImage {
        let maximum = CGSize(width: 640, height: 360)
        let scale = min(maximum.width / image.size.width, maximum.height / image.size.height, 1)
        guard scale < 1 else { return image }

        let size = CGSize(width: image.size.width * scale, height: image.size.height * scale)
        return UIGraphicsImageRenderer(size: size).image { _ in
            image.draw(in: CGRect(origin: .zero, size: size))
        }
    }
}
