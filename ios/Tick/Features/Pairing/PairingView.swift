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
            BannerView()
            Text("your projects' next steps,\nsynced with the tick CLI on your Mac.")
                .font(.statusLine)
                .foregroundStyle(Theme.inkSecondary)
                .multilineTextAlignment(.center)
            Spacer()
            VStack(spacing: Theme.Space.base) {
                if DataScannerViewController.isSupported {
                    Button {
                        showScanner = true
                    } label: {
                        Label("scan pairing code", systemImage: "qrcode.viewfinder")
                            .font(.mono(.body, .semibold))
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderedProminent)
                    .foregroundStyle(Theme.bg)
                    .controlSize(.large)
                    Text("run  tick sync qr  in your terminal")
                        .font(.statusLine)
                        .foregroundStyle(Theme.inkDim)
                }
                VStack(spacing: Theme.Space.hair) {
                    HStack(spacing: Theme.Space.tight) {
                        Text("pair:")
                            .font(.mono(.body, .semibold))
                            .foregroundStyle(Theme.cyan)
                        TextField("", text: $pasted, prompt: Text("…or paste the tick://pair link").foregroundStyle(Theme.inkDim))
                            .font(.statusLine)
                            .foregroundStyle(Theme.ink)
                            .autocorrectionDisabled()
                            .textInputAutocapitalization(.never)
                            .onSubmit(usePasted)
                    }
                    .padding(.horizontal, Theme.Space.base)
                    .padding(.vertical, Theme.Space.snug)
                    .background(Theme.surface, in: RoundedRectangle(cornerRadius: 14))
                    .overlay(RoundedRectangle(cornerRadius: 14).stroke(Theme.rule.opacity(0.5), lineWidth: 1))
                    if pasteError {
                        Text("that doesn't look like a tick://pair link")
                            .font(.statusLine)
                            .foregroundStyle(Theme.pink)
                    }
                }
            }
            .padding(.horizontal, Theme.Space.roomy)
            Spacer()
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Theme.bg)
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
