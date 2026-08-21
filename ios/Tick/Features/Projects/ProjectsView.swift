import SwiftUI

/// The overview: the terminal banner, then every project as one card, grouped
/// like the TUI. Projects are born on the Mac (that's where the folder link
/// lives) — here you pick one and go.
struct ProjectsView: View {
    @Environment(AppModel.self) private var app
    @State private var showSettings = false
    @State private var editing: ProjectForm?
    @State private var confirmDelete: Project?

    private var doc: StoreDocument { app.store.doc }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Theme.Space.roomy) {
                header
                if doc.liveProjects.isEmpty {
                    emptyState
                } else {
                    ForEach(doc.sections, id: \.group) { section in
                        VStack(alignment: .leading, spacing: Theme.Space.snug) {
                            if let g = section.group {
                                GroupHeader(title: g)
                            } else if doc.sections.count > 1 {
                                GroupHeader(title: "ungrouped")
                            }
                            ForEach(section.projects) { project in
                                ProjectCard(project: project, doc: doc)
                                    .contextMenu {
                                        Button {
                                            editing = ProjectForm(project: project)
                                        } label: {
                                            Label("Rename / group", systemImage: "pencil")
                                        }
                                        Button(role: .destructive) {
                                            confirmDelete = project
                                        } label: {
                                            Label("Untrack", systemImage: "trash")
                                        }
                                    }
                            }
                        }
                    }
                }
            }
            .padding(.horizontal, Theme.Space.base)
            .padding(.bottom, Theme.Space.roomy)
        }
        .background(Theme.bg)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button { showSettings = true } label: { Image(systemName: "gearshape") }
            }
        }
        .toolbarBackground(.hidden, for: .navigationBar)
        .refreshable { await app.engine.syncNow() }
        .safeAreaInset(edge: .bottom) { SyncStatusLine() }
        .sheet(isPresented: $showSettings) { SettingsView() }
        .sheet(item: $editing) { form in ProjectFormSheet(form: form) }
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

    private var header: some View {
        VStack(alignment: .leading, spacing: Theme.Space.tight) {
            BannerView()
            HStack(alignment: .firstTextBaseline) {
                Text("next steps across your projects")
                    .font(.statusLine)
                    .foregroundStyle(Theme.inkDim)
                Spacer()
                let n = doc.liveProjects.count
                Text("\(n) project\(n == 1 ? "" : "s")")
                    .font(.countLabel)
                    .foregroundStyle(Theme.yellow)
            }
        }
        .padding(.top, Theme.Space.tight)
    }

    private var emptyState: some View {
        VStack(alignment: .leading, spacing: Theme.Space.snug) {
            Text("no projects yet")
                .font(.projectName)
                .foregroundStyle(Theme.inkSecondary)
            Text("cd into one on your Mac and `tick add` a step — it shows up here after the next sync.")
                .font(.statusLine)
                .foregroundStyle(Theme.inkDim)
        }
        .padding(Theme.Space.base)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Theme.surface, in: RoundedRectangle(cornerRadius: 14))
    }
}

struct GroupHeader: View {
    let title: String
    var body: some View {
        Text(title.uppercased())
            .font(.groupHeader)
            .kerning(1.4)
            .foregroundStyle(Theme.purple)
            .padding(.top, Theme.Space.tight)
    }
}

/// One project: name, a peek at its first next step, the open count in the
/// TUI's count-yellow.
struct ProjectCard: View {
    let project: Project
    let doc: StoreDocument

    var body: some View {
        NavigationLink(value: project.id) {
            HStack(alignment: .center, spacing: Theme.Space.snug) {
                VStack(alignment: .leading, spacing: Theme.Space.hair) {
                    Text(project.name)
                        .font(.projectName)
                        .foregroundStyle(Theme.ink)
                        .lineLimit(1)
                    if let next = doc.steps(of: project.id, includeDone: false).first {
                        HStack(spacing: Theme.Space.hair) {
                            Text("›")
                                .foregroundStyle(Theme.cyan)
                            Text(next.text)
                                .foregroundStyle(Theme.inkSecondary)
                                .lineLimit(1)
                        }
                        .font(.statusLine)
                    } else {
                        Text("all clear")
                            .font(.statusLine)
                            .foregroundStyle(Theme.inkDim)
                    }
                }
                Spacer(minLength: Theme.Space.tight)
                OpenCountLabel(open: doc.openCount(of: project.id))
                Image(systemName: "chevron.right")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(Theme.inkDim)
            }
            .padding(.horizontal, Theme.Space.base)
            .padding(.vertical, Theme.Space.snug + 2)
            .background(Theme.surface, in: RoundedRectangle(cornerRadius: 14))
            .overlay(RoundedRectangle(cornerRadius: 14).stroke(Theme.rule.opacity(0.45), lineWidth: 1))
            .contentShape(RoundedRectangle(cornerRadius: 14))
        }
        .buttonStyle(.plain)
    }
}

/// "3 open" in count-yellow, or a dim dash.
struct OpenCountLabel: View {
    let open: Int
    var body: some View {
        Text(open == 0 ? "—" : "\(open) open")
            .font(.countLabel)
            .foregroundStyle(open == 0 ? Theme.inkDim : Theme.yellow)
    }
}

/// Rename / regroup an existing project. Creation stays on the Mac.
struct ProjectForm: Identifiable {
    var id: String { projectID }
    var projectID: String
    var name: String
    var group: String

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
            .scrollContentBackground(.hidden)
            .background(Theme.bg)
            .navigationTitle("Edit project")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") {
                        let name = form.name.trimmingCharacters(in: .whitespaces)
                        guard !name.isEmpty else { return }
                        app.store.editProject(form.projectID, name: name, group: form.group)
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
                    Text("⇅ syncing…")
                }
            case .offline:
                Text("⇅ offline — saved on this phone")
            case .authFailed:
                Text("⇅ token rejected — re-pair in settings")
                    .foregroundStyle(Theme.pink)
            case .idle:
                if let t = app.engine.lastSynced {
                    Text("⇅ synced \(t.formatted(.relative(presentation: .named)))")
                } else {
                    Text(" ")
                }
            }
        }
        .font(.statusLine)
        .foregroundStyle(Theme.inkDim)
        .frame(maxWidth: .infinity)
        .padding(.vertical, Theme.Space.hair)
        .background(Theme.bg.opacity(0.9))
    }
}
