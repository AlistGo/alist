import AVKit
import SwiftUI

struct PlayerView: View {
    @ObservedObject var coordinator: PlayerCoordinator
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        Group {
            if let adapter = coordinator.controller as? AVPlayerControllerAdapter {
                AVPlayerControllerView(player: adapter.player)
                    .ignoresSafeArea()
            } else {
                VStack(spacing: 32) {
                    Image(systemName: "play.rectangle.fill")
                        .font(.system(size: 96))
                    Text("Fixture Player")
                        .font(.largeTitle)
                    Button("Done") {
                        coordinator.playerDidDisappear()
                        dismiss()
                    }
                    .accessibilityIdentifier("player.done")
                }
            }
        }
        .overlay(alignment: .bottom) {
            if case let .failed(message) = coordinator.state {
                Text(message)
                    .padding(24)
                    .background(.ultraThinMaterial, in: RoundedRectangle(cornerRadius: 16))
                    .padding(50)
            }
        }
        .onExitCommand {
            coordinator.playerDidDisappear()
            dismiss()
        }
        .onDisappear { coordinator.playerDidDisappear() }
    }
}

struct AVPlayerControllerView: UIViewControllerRepresentable {
    let player: AVPlayer

    func makeUIViewController(context: Context) -> AVPlayerViewController {
        let controller = AVPlayerViewController()
        controller.player = player
        controller.showsPlaybackControls = true
        return controller
    }

    func updateUIViewController(_ controller: AVPlayerViewController, context: Context) {
        if controller.player !== player { controller.player = player }
    }
}
