import AVFoundation
import Foundation

enum PlayerEvent: Equatable, Sendable {
    case failed(message: String)
    case paused
}

@MainActor
protocol PlayerControlling: AnyObject {
    var events: AsyncStream<PlayerEvent> { get }
    var currentTime: TimeInterval { get }
    var duration: TimeInterval { get }

    func replaceCurrentItem(url: URL)
    func play()
    func pause()
    func seek(to seconds: TimeInterval) async
}

@MainActor
final class AVPlayerControllerAdapter: NSObject, PlayerControlling {
    let player = AVPlayer()
    let events: AsyncStream<PlayerEvent>

    private var continuation: AsyncStream<PlayerEvent>.Continuation!
    private var itemObservation: NSKeyValueObservation?
    private var timeControlObservation: NSKeyValueObservation?

    override init() {
        var captured: AsyncStream<PlayerEvent>.Continuation!
        events = AsyncStream { captured = $0 }
        continuation = captured
        super.init()
        timeControlObservation = player.observe(\.timeControlStatus, options: [.new]) { [weak self] player, _ in
            guard player.timeControlStatus == .paused else { return }
            Task { @MainActor [weak self] in self?.continuation.yield(.paused) }
        }
    }

    var currentTime: TimeInterval {
        let seconds = player.currentTime().seconds
        return seconds.isFinite ? max(0, seconds) : 0
    }

    var duration: TimeInterval {
        guard let item = player.currentItem else { return 0 }
        let seconds = item.duration.seconds
        return seconds.isFinite ? seconds : 0
    }

    func replaceCurrentItem(url: URL) {
        itemObservation?.invalidate()
        let item = AVPlayerItem(url: url)
        itemObservation = item.observe(\.status, options: [.new]) { [weak self] item, _ in
            guard item.status == .failed else { return }
            let message = item.error?.localizedDescription ?? "Playback failed."
            Task { @MainActor [weak self] in self?.continuation.yield(.failed(message: message)) }
        }
        player.replaceCurrentItem(with: item)
    }

    func play() {
        player.play()
    }

    func pause() {
        player.pause()
    }

    func seek(to seconds: TimeInterval) async {
        let time = CMTime(seconds: max(0, seconds), preferredTimescale: 600)
        await withCheckedContinuation { continuation in
            player.seek(to: time, toleranceBefore: .zero, toleranceAfter: .zero) { _ in
                continuation.resume()
            }
        }
    }

    deinit {
        itemObservation?.invalidate()
        timeControlObservation?.invalidate()
        continuation.finish()
    }
}
