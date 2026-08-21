import SwiftUI
import VisionKit

/// First-run screen: pair with the Mac. Scan the QR from `tick sync qr`, or
/// paste the same tick:// link. Credentials are only stored after the
/// confirmation sheet.
struct PairingView: View {
    @Environment(AppModel.self) private var app
    @State private var showScanner = false
    @State private var pasted = ""
    @State private var pasteError = false

    var body: some View {
        VStack(spacing: Theme.Space.roomy) {
            Spacer()
            Image(systemName: "checkmark.circle")
                .font(.system(size: 56, weight: .light))
                .foregroundStyle(.tint)
            VStack(spacing: Theme.Space.tight) {
                Text("Tick")
                    .font(.largeTitle.bold())
                Text("Your projects' next steps,\nsynced with the tick CLI on your Mac.")
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
            }
            Spacer()
            VStack(spacing: Theme.Space.base) {
                if DataScannerViewController.isSupported {
                    Button {
                        showScanner = true
                    } label: {
                        Label("Scan pairing code", systemImage: "qrcode.viewfinder")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderedProminent)
                    .controlSize(.large)
                    Text("Run `tick sync qr` in your terminal")
                        .font(.statusLine)
                        .foregroundStyle(.secondary)
                }
                VStack(spacing: Theme.Space.hair) {
                    TextField("…or paste the tick://pair link", text: $pasted)
                        .textFieldStyle(.roundedBorder)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                        .onSubmit(usePasted)
                    if pasteError {
                        Text("That doesn't look like a tick://pair link.")
                            .font(.footnote)
                            .foregroundStyle(.orange)
                    }
                }
            }
            .padding(.horizontal, Theme.Space.roomy)
            Spacer()
        }
        .sheet(isPresented: $showScanner) {
            QRScannerSheet { payload in
                showScanner = false
                handle(payload)
            }
        }
    }

    private func usePasted() {
        handle(pasted.trimmingCharacters(in: .whitespacesAndNewlines))
    }

    private func handle(_ payload: String) {
        guard let url = URL(string: payload), case .pair(let server, let token)? = DeepLink(url) else {
            pasteError = true
            return
        }
        pasteError = false
        app.pendingPair = PairRequest(server: server, token: token)
    }
}
