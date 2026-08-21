import Foundation
import SwiftUI

struct ConnectionView: View {
    @ObservedObject var viewModel: ConnectionViewModel
    let submit: @MainActor (String, String, String) async -> Void
    let submitOTP: @MainActor (String) async -> Void
    let retryRecovery: @MainActor () async -> Void

    @State private var serverURL = ""
    @State private var username = ""
    @State private var password = ""
    @State private var otpCode = ""

    init(
        viewModel: ConnectionViewModel,
        submit: @escaping @MainActor (String, String, String) async -> Void,
        submitOTP: @escaping @MainActor (String) async -> Void,
        retryRecovery: @escaping @MainActor () async -> Void
    ) {
        self.viewModel = viewModel
        self.submit = submit
        self.submitOTP = submitOTP
        self.retryRecovery = retryRecovery
#if DEBUG
        let arguments = ProcessInfo.processInfo.arguments
        if arguments.contains("ui-testing") {
            func value(for key: String) -> String {
                let prefix = "\(key)="
                guard let argument = arguments.first(where: { $0.hasPrefix(prefix) }) else { return "" }
                return String(argument.dropFirst(prefix.count))
            }
            _serverURL = State(initialValue: value(for: "ui-server"))
            _username = State(initialValue: value(for: "ui-username"))
            _password = State(initialValue: value(for: "ui-password"))
            _otpCode = State(initialValue: value(for: "ui-otp"))
        }
#endif
    }

    var body: some View {
        VStack(spacing: 28) {
            Text("AList")
                .font(.largeTitle.bold())

            if viewModel.state == .otpRequired {
                Text("Two-factor authentication")
                    .font(.title2)
                TextField("One-time code", text: $otpCode)
                    .textContentType(.oneTimeCode)
                    .onSubmit {
                        Task {
                            await submitOTP(otpCode)
                            otpCode = ""
                        }
                    }
                    .accessibilityIdentifier("connection.otp")
                HStack(spacing: 28) {
                    Button("Cancel") {
                        password = ""
                        otpCode = ""
                        viewModel.cancelOTP()
                    }
                    Button("Verify") {
                        Task {
                            await submitOTP(otpCode)
                            otpCode = ""
                        }
                    }
                    .accessibilityIdentifier("connection.verify")
                }
            } else {
                TextField("https://alist.example.com", text: $serverURL)
                    .textContentType(.URL)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("connection.server")
                TextField("Username", text: $username)
                    .textContentType(.username)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("connection.username")
                SecureField("Password", text: $password)
                    .textContentType(.password)
                    .accessibilityIdentifier("connection.password")
                Button(viewModel.state == .submitting ? "Connecting…" : "Connect") {
                    Task {
                        await submit(serverURL, username, password)
                        if viewModel.state == .connected { password = "" }
                    }
                }
                .disabled(viewModel.state == .submitting)
                .accessibilityIdentifier("connection.connect")
            }

            if case let .failed(message) = viewModel.state {
                Text(message)
                    .foregroundStyle(.red)
                    .multilineTextAlignment(.center)
                    .accessibilityIdentifier("connection.error")
                if viewModel.canRetryRecovery {
                    Button("Retry session") {
                        Task { await retryRecovery() }
                    }
                    .accessibilityIdentifier("connection.retry-session")
                }
            }
        }
        .frame(maxWidth: 760)
        .padding(80)
    }
}
