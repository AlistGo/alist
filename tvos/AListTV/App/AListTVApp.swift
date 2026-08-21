import SwiftUI

@main
struct AListTVApp: App {
    @StateObject private var container = AppContainer()

    var body: some Scene {
        WindowGroup {
            AppRootView(container: container)
        }
    }
}

private struct AppRootView: View {
    @ObservedObject var container: AppContainer

    var body: some View {
        Group {
            switch container.route {
            case .connection:
                ConnectionView(
                    viewModel: container.connectionViewModel,
                    submit: { serverURL, username, password in
                        await container.submit(serverURL: serverURL, username: username, password: password)
                    },
                    submitOTP: { code in await container.submitOTP(code) },
                    retryRecovery: { await container.retrySessionRecovery() }
                )
            case .browser:
                if let browser = container.browserViewModel,
                   let player = container.playerCoordinator {
                    BrowserHost(
                        browser: browser,
                        player: player,
                        onLogout: container.logout,
                        logoutError: container.logoutError,
                        dismissLogoutError: container.dismissLogoutError
                    )
                } else {
                    ProgressView()
                }
            }
        }
        .task { await container.restoreSession() }
    }
}

private struct BrowserHost: View {
    @ObservedObject var browser: BrowserViewModel
    @ObservedObject var player: PlayerCoordinator
    let onLogout: () -> Void
    let logoutError: String?
    let dismissLogoutError: () -> Void

    var body: some View {
        BrowserView(
            viewModel: browser,
            onLogout: onLogout,
            logoutError: logoutError,
            dismissLogoutError: dismissLogoutError
        )
            .fullScreenCover(isPresented: $player.isPresented) {
                PlayerView(coordinator: player)
            }
    }
}
