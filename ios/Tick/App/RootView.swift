import SwiftUI

struct RootView: View {
    @Environment(AppModel.self) private var app

    var body: some View {
        @Bindable var app = app
        Group {
            if app.settings.isPaired {
                NavigationStack(path: $app.path) {
                    ProjectsView()
                        .navigationDestination(for: String.self) { id in
                            StepsView(projectID: id)
                        }
                }
            } else {
                PairingView()
            }
        }
        .task { app.restoreLastProject() }
        .sheet(item: $app.pendingPair) { req in
            PairConfirmSheet(request: req)
        }
    }
}

/// The one gate between a scanned/tapped link and stored credentials.
struct PairConfirmSheet: View {
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    let request: PairRequest

    var body: some View {
        VStack(spacing: Theme.Space.roomy) {
            Image(systemName: "link")
                .font(.largeTitle)
                .foregroundStyle(.tint)
            Text(app.settings.isPaired ? "Replace pairing?" : "Pair with your Mac?")
                .font(.title2.bold())
            Text(request.server.host() ?? request.server.absoluteString)
                .font(.statusLine)
                .foregroundStyle(.secondary)
            if app.settings.isPaired {
                Text("This replaces the current server and token.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            Button {
                app.confirmPair(request)
                dismiss()
            } label: {
                Text("Connect")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            Button("Cancel") { dismiss() }
                .foregroundStyle(.secondary)
        }
        .padding(Theme.Space.roomy)
        .presentationDetents([.medium])
    }
}
