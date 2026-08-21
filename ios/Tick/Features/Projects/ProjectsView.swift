import SwiftUI

/// The overview: every project, grouped like the TUI, each row one calm line.
/// The point of the app is to pick ONE of these and see nothing else.
struct ProjectsView: View {
    @Environment(AppModel.self) private var app
    @State private var showSettings = false
    @State private var editing: ProjectForm?
    @State private var confirmDelete: Project?

    var body: some View {
        List {
            ForEach(app.store.doc.sections, id: \.group) { section in
                Section {
                    ForEach(section.projects) { project in
                        row(project)
                    }
                } header: {
                    if let g = section.group {
                        Text(g.uppercased())
                            .font(.caption.weight(.semibold))
                            .kerning(0.8)
                    } else if app.store.doc.sections.count > 1 {
                        Text("UNGROUPED")
                            .font(.caption.weight(.semibold))
                            .kerning(0.8)
                    }
                }
            }
        }
        .navigationTitle("Tick")
        .overlay {
            if app.store.doc.liveProjects.isEmpty {
                ContentUnavailableView {
                    Label("No projects yet", systemImage: "checkmark.circle")
                } description: {
                    Text("Add one here, or `tick add` on your Mac — it shows up after the next sync.")
                }
            }
        }
        .toolbar {
            ToolbarItem(placement: .topBarLeading) {
                Button { showSettings = true } label: { Image(systemName: "gearshape") }
            }
            ToolbarItem(placement: .topBarTrailing) {
                Button { editing = ProjectForm() } label: { Image(systemName: "plus") }
            }
        }
        .refreshable { await app.engine.syncNow() }
        .safeAreaInset(edge: .bottom) { SyncStatusLine() }
        .sheet(isPresented: $showSettings) { SettingsView() }
        .sheet(item: $editing) { form in
            ProjectFormSheet(form: form)
        }
        .confirmationDialog(
            "Untrack \u{201C}\(confirmDelete?.name ?? "")\u{201D}?",
            isPresented: Binding(get: { confirmDelete != nil }, set: { if !$0 { confirmDelete = nil } }),
            titleVisibility: .visible
        ) {
            Button("Untrack and delete its steps", role: .destructive) {
                if let p = confirmDelete {
                    Haptics.delete()
                    app.store.deleteProject(p.id)
                }
                confirmDelete = nil
            }
        } message: {
            Text("Removes the project and its steps on every device.")
        }
    }

    private func row(_ project: Project) -> some View {
        NavigationLink(value: project.id) {
            HStack {
                Text(project.name)
                    .fontWeight(.medium)
                Spacer()
                let open = app.store.doc.openCount(of: project.id)
                Text(open == 0 ? "—" : "\(open) open")
                    .font(.countLabel)
                    .foregroundStyle(open == 0 ? .tertiary : .secondary)
            }
            .padding(.vertical, Theme.Space.hair)
        }
        .swipeActions(edge: .leading) {
            Button {
                editing = ProjectForm(project: project)
            } label: {
                Label("Edit", systemImage: "pencil")
            }
            .tint(.accentColor)
        }
        .swipeActions(edge: .trailing) {
            Button(role: .destructive) {
                confirmDelete = project
            } label: {
                Label("Untrack", systemImage: "trash")
            }
        }
    }
}

/// New-or-edit form state.
struct ProjectForm: Identifiable {
    var id: String { projectID ?? "new" }
    var projectID: String?
    var name = ""
    var group = ""

    init() {}
    init(project: Project) {
        projectID = project.id
        name = project.name
        group = project.group ?? ""
    }
}

struct ProjectFormSheet: View {
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    @State var form: ProjectForm
    @FocusState private var nameFocused: Bool

    var body: some View {
        NavigationStack {
            Form {
                TextField("Name", text: $form.name)
                    .focused($nameFocused)
                TextField("Group (optional)", text: $form.group)
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
            }
            .navigationTitle(form.projectID == nil ? "New project" : "Edit project")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") {
                        let name = form.name.trimmingCharacters(in: .whitespaces)
                        guard !name.isEmpty else { return }
                        if let id = form.projectID {
                            app.store.editProject(id, name: name, group: form.group)
                        } else {
                            let p = app.store.addProject(name: name, group: form.group)
                            app.path = [p.id]
                        }
                        dismiss()
                    }
                    .disabled(form.name.trimmingCharacters(in: .whitespaces).isEmpty)
                }
            }
        }
        .presentationDetents([.medium])
        .onAppear { nameFocused = true }
    }
}

/// One quiet mono line about sync — informative, never nagging.
struct SyncStatusLine: View {
    @Environment(AppModel.self) private var app

    var body: some View {
        Group {
            switch app.engine.status {
            case .syncing:
                HStack(spacing: Theme.Space.tight) {
                    ProgressView().controlSize(.mini)
                    Text("Syncing…")
                }
            case .offline:
                Text("Offline — saved on this phone")
            case .authFailed:
                Text("Token rejected — re-pair in Settings")
                    .foregroundStyle(.orange)
            case .idle:
                if let t = app.engine.lastSynced {
                    Text("Synced \(t.formatted(.relative(presentation: .named)))")
                } else {
                    Text(" ")
                }
            }
        }
        .font(.statusLine)
        .foregroundStyle(.secondary)
        .frame(maxWidth: .infinity)
        .padding(.vertical, Theme.Space.hair)
    }
}
