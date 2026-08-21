import SwiftUI
import UIKit

struct BrowserView: View {
    @ObservedObject var viewModel: BrowserViewModel
    let onLogout: () -> Void
    let logoutError: String?
    let dismissLogoutError: () -> Void


    @FocusState private var focusedPath: String?

    private let columns = [
        GridItem(.adaptive(minimum: 280, maximum: 360), spacing: 36)
    ]

    var body: some View {
        NavigationStack {
            Group {
                switch viewModel.state {
                case .idle, .loading:
                    focusableStatus(title: "Loading…", identifier: "browser.loading")
                case .empty:
                    focusableStatus(title: "Empty folder", identifier: "browser.empty")
                case .failed:
                    retryState(title: "Unable to load this folder")
                case .forbidden where viewModel.items.isEmpty:
                    retryState(title: "You do not have permission to open this folder")
                default:
                    grid
                }
            }
            .navigationTitle(viewModel.path)
            .toolbar {
                if viewModel.path != "/" {
                    ToolbarItem(placement: .topBarLeading) {
                        Button("Back") { viewModel.moveToParent() }
                    }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Sign Out", action: onLogout)
                        .accessibilityIdentifier("browser.logout")
                }
            }
        }
        .alert(
            "Unable to sign out",
            isPresented: Binding(
                get: { logoutError != nil },
                set: { if !$0 { dismissLogoutError() } }
            )
        ) {
            Button("OK", role: .cancel, action: dismissLogoutError)
        } message: {
            Text(logoutError ?? "")
        }
        .onAppear {
            if viewModel.state == .idle { viewModel.loadInitial() }
        }
        .onChange(of: focusedPath) { _, value in
            viewModel.focusedVirtualPath = value
            guard let value,
                  let item = viewModel.items.first(where: { $0.virtualPath == value }) else { return }
            viewModel.loadNextPageIfNeeded(currentItem: item)
        }
        .onChange(of: viewModel.focusedVirtualPath) { _, value in
            if value != nil { focusedPath = value }
        }
        .onExitCommand { viewModel.moveToParent() }
    }

    private var grid: some View {
        ScrollView {
            LazyVGrid(columns: columns, spacing: 42) {
                ForEach(viewModel.items) { item in
                    Button {
                        viewModel.open(item)
                    } label: {
                        BrowserCard(
                            item: item,
                            artwork: viewModel.artwork(for: item),
                            loadArtwork: { viewModel.loadArtwork(for: item) }
                        )
                    }
                    .buttonStyle(.card)
                    .focused($focusedPath, equals: item.virtualPath)
                    .accessibilityIdentifier("browser.item.\(item.virtualPath ?? item.name)")
                    .accessibilityValue(focusedPath == item.virtualPath ? "focused" : "")
                }

                switch viewModel.state {
                case .loadingMore:
                    focusableStatus(title: "Loading more…", identifier: "browser.loading-more")
                case .loadMoreFailed:
                    Button("Retry loading more") { viewModel.retry() }
                        .accessibilityIdentifier("browser.retry-more")
                case .forbidden:
                    Button("Permission denied — retry") { viewModel.retry() }
                        .accessibilityIdentifier("browser.retry-forbidden")
                default:
                    EmptyView()
                }
            }
            .padding(60)
        }
    }

    private func retryState(title: String) -> some View {
        VStack(spacing: 28) {
            Text(title).font(.title2)
            Button("Retry") { viewModel.retry() }
                .accessibilityIdentifier("browser.retry")
        }
    }

    private func focusableStatus(title: String, identifier: String) -> some View {
        Button(title) {}
            .accessibilityIdentifier(identifier)
    }
}

private struct BrowserCard: View {
    let item: AListObject
    let artwork: UIImage?
    let loadArtwork: () -> Void


    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            thumbnail
                .frame(height: 170)
                .frame(maxWidth: .infinity)
                .background(.quaternary, in: RoundedRectangle(cornerRadius: 18))
                .clipShape(RoundedRectangle(cornerRadius: 18))
            Text(item.name)
                .font(.headline)
                .lineLimit(2)
            Text(item.isDirectory ? "Folder" : ByteCountFormatter.string(fromByteCount: item.size, countStyle: .file))
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .padding(18)
    }

    @ViewBuilder
    private var thumbnail: some View {
        if let artwork {
            Image(uiImage: artwork)
                .resizable()
                .scaledToFill()
        } else if let value = item.thumbnail,
                  let components = URLComponents(string: value),
                  components.scheme?.lowercased() == "https",
                  components.host != nil,
                  let url = components.url {
            AsyncImage(url: url) { phase in
                switch phase {
                case let .success(image):
                    image.resizable().scaledToFill()
                case .failure:
                    placeholder.onAppear(perform: loadArtwork)
                case .empty:
                    placeholder
                @unknown default:
                    placeholder
                }
            }
        } else if item.fileType == .audio || item.fileType == .video {
            placeholder
                .onAppear(perform: loadArtwork)
        } else {
            placeholder
        }
    }

    private var placeholder: some View {
        Image(systemName: item.isDirectory ? "folder.fill" : symbolForType)
            .font(.system(size: 64))
            .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var symbolForType: String {
        switch item.fileType {
        case .folder:
            return "folder.fill"
        case .video:
            return "play.rectangle.fill"
        case .audio:
            return "music.note"
        case .image:
            return "photo.fill"
        default:
            return "doc.fill"
        }
    }
}
