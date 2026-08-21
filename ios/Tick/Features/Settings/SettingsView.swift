import SwiftUI

struct SettingsView: View {
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    @State private var confirmUnpair = false
    @State private var showRepair = false

    var body: some View {
        @Bindable var settings = app.settings
        NavigationStack {
            Form {
                Section("Server") {
                    LabeledContent("Address", value: app.settings.baseURL?.host() ?? "—")
                    LabeledContent("This device", value: app.store.doc.device)
                    LabeledContent("Status") { SyncStatusLine() }
                    if app.store.pendingPushCount > 0 {
                        LabeledContent("Queued", value: "\(app.store.pendingPushCount) change(s)")
                    }
                    Button("Sync now") {
                        Task { await app.engine.syncNow() }
                    }
                }
                Section("Display") {
                    Toggle("Show done steps by default", isOn: $settings.showDoneByDefault)
                }
                Section("Data") {
                    Button("Re-sync everything") {
                        Task { await app.engine.fullResync() }
                    }
                }
                Section("Pairing") {
                    Button("Re-pair with a new code") { showRepair = true }
                    Button("Unpair", role: .destructive) { confirmUnpair = true }
                }
                Section {
                    LabeledContent("Version", value: Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "dev")
                } footer: {
                    Text("Steps live on this phone and sync through your own server — no accounts, one token.")
                }
            }
            .navigationTitle("Settings")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                }
            }
            .confirmationDialog("Unpair from the server?", isPresented: $confirmUnpair, titleVisibility: .visible) {
                Button("Unpair — keep steps on this phone", role: .destructive) {
                    app.settings.unpair()
                    dismiss()
                }
            } message: {
                Text("Your steps stay here; they just stop syncing until you pair again.")
            }
            .sheet(isPresented: $showRepair) {
                NavigationStack { PairingView() }
                    .presentationDetents([.large])
            }
        }
    }
}
