import SwiftUI
import VisionKit

/// VisionKit's scanner wrapped for the one-shot pairing scan: first QR code
/// recognized wins. ~40 lines instead of an AVFoundation session, with
/// focus/highlighting for free. (The Simulator has no camera; PairingView
/// offers the paste fallback there.)
struct QRScannerSheet: View {
    let found: (String) -> Void
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            Group {
                if DataScannerViewController.isAvailable {
                    QRScannerView(found: found)
                } else {
                    ContentUnavailableView {
                        Label("Camera unavailable", systemImage: "camera.on.rectangle")
                    } description: {
                        Text("Allow camera access in Settings, or paste the pairing link instead.")
                    }
                }
            }
            .navigationTitle("Scan")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
            }
        }
    }
}

private struct QRScannerView: UIViewControllerRepresentable {
    let found: (String) -> Void

    func makeUIViewController(context: Context) -> DataScannerViewController {
        let vc = DataScannerViewController(
            recognizedDataTypes: [.barcode(symbologies: [.qr])],
            qualityLevel: .fast,
            isHighlightingEnabled: true
        )
        vc.delegate = context.coordinator
        try? vc.startScanning()
        return vc
    }

    func updateUIViewController(_ vc: DataScannerViewController, context: Context) {}

    func makeCoordinator() -> Coordinator { Coordinator(found: found) }

    final class Coordinator: NSObject, DataScannerViewControllerDelegate {
        let found: (String) -> Void
        private var fired = false

        init(found: @escaping (String) -> Void) {
            self.found = found
        }

        func dataScanner(_ scanner: DataScannerViewController, didAdd added: [RecognizedItem], allItems: [RecognizedItem]) {
            guard !fired else { return }
            for item in added {
                if case .barcode(let code) = item, let payload = code.payloadStringValue {
                    fired = true
                    found(payload)
                    return
                }
            }
        }
    }
}
