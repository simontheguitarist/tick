import AppIntents
import SwiftUI
import WidgetKit

@main
struct TickWidgetBundle: WidgetBundle {
    var body: some Widget {
        NextStepsWidget()
    }
}

// MARK: - Configuration (pick a project; default = most recently active)

struct ProjectEntity: AppEntity {
    static let typeDisplayRepresentation: TypeDisplayRepresentation = "Project"
    static let defaultQuery = ProjectQuery()

    var id: String
    var name: String

    var displayRepresentation: DisplayRepresentation {
        DisplayRepresentation(title: "\(name)")
    }
}

struct ProjectQuery: EntityQuery {
    func entities(for identifiers: [String]) async throws -> [ProjectEntity] {
        all().filter { identifiers.contains($0.id) }
    }

    func suggestedEntities() async throws -> [ProjectEntity] {
        all()
    }

    private func all() -> [ProjectEntity] {
        (StoreFile.appGroup().read()?.liveProjects ?? []).map { ProjectEntity(id: $0.id, name: $0.name) }
    }
}

struct SelectProjectIntent: WidgetConfigurationIntent {
    static let title: LocalizedStringResource = "Next steps"
    static let description = IntentDescription("Shows a project's next open steps.")

    @Parameter(title: "Project")
    var project: ProjectEntity?
}

// MARK: - Timeline

struct StepLine: Identifiable {
    let id: String
    let number: Int
    let text: String
    let important: Bool
}

struct NextStepsEntry: TimelineEntry {
    let date: Date
    let projectID: String?
    let projectName: String?
    let steps: [StepLine]
    let openCount: Int
}

struct NextStepsProvider: AppIntentTimelineProvider {
    func placeholder(in context: Context) -> NextStepsEntry {
        NextStepsEntry(date: .now, projectID: nil, projectName: "blitzsheet",
                       steps: [StepLine(id: "1", number: 1, text: "Plan the next release", important: true),
                               StepLine(id: "2", number: 2, text: "Fix the login flow", important: false)],
                       openCount: 2)
    }

    func snapshot(for configuration: SelectProjectIntent, in context: Context) async -> NextStepsEntry {
        entry(for: configuration)
    }

    func timeline(for configuration: SelectProjectIntent, in context: Context) async -> Timeline<NextStepsEntry> {
        // The app reloads all timelines after every persist, so this snapshot
        // is fresh by construction — no timed refresh needed.
        Timeline(entries: [entry(for: configuration)], policy: .never)
    }

    private func entry(for configuration: SelectProjectIntent) -> NextStepsEntry {
        guard let doc = StoreFile.appGroup().read() else {
            return NextStepsEntry(date: .now, projectID: nil, projectName: nil, steps: [], openCount: 0)
        }
        let project = configuration.project.flatMap { doc.project($0.id) } ?? doc.mostRecentlyActiveProject
        guard let project else {
            return NextStepsEntry(date: .now, projectID: nil, projectName: nil, steps: [], openCount: 0)
        }
        let open = doc.steps(of: project.id, includeDone: false)
        let lines = open.prefix(3).enumerated().map { i, s in
            StepLine(id: s.id, number: i + 1, text: s.text, important: s.important)
        }
        return NextStepsEntry(date: .now, projectID: project.id, projectName: project.name,
                              steps: lines, openCount: open.count)
    }
}

// MARK: - Views

struct NextStepsWidgetView: View {
    @Environment(\.widgetFamily) private var family
    let entry: NextStepsEntry

    var body: some View {
        Group {
            if let name = entry.projectName {
                content(name)
            } else {
                VStack(spacing: 6) {
                    Image(systemName: "checkmark.circle")
                        .font(.title3)
                        .foregroundStyle(.tint)
                    Text("Open Tick to get started")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .multilineTextAlignment(.center)
                }
            }
        }
        .containerBackground(for: .widget) { Color("WidgetBackground") }
        .widgetURL(entry.projectID.flatMap { URL(string: "tick://project/\($0)") })
    }

    private func content(_ name: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline) {
                Text(name)
                    .font(.headline)
                    .lineLimit(1)
                Spacer(minLength: 4)
                Text(entry.openCount == 0 ? "—" : "\(entry.openCount)")
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(.secondary)
            }
            if entry.steps.isEmpty {
                Spacer()
                HStack(spacing: 6) {
                    Image(systemName: "checkmark")
                        .foregroundStyle(.tint)
                    Text("All clear")
                        .foregroundStyle(.secondary)
                }
                .font(.subheadline)
                Spacer()
            } else {
                let shown = family == .systemSmall ? Array(entry.steps.prefix(1)) : entry.steps
                ForEach(shown) { line in
                    HStack(alignment: .firstTextBaseline, spacing: 6) {
                        Text("\(line.number).")
                            .font(.system(.caption, design: .monospaced).weight(.medium))
                            .foregroundStyle(line.important ? AnyShapeStyle(.tint) : AnyShapeStyle(.secondary))
                        Text(line.text)
                            .font(family == .systemSmall ? .subheadline : .caption)
                            .fontWeight(line.important ? .semibold : .regular)
                            .foregroundStyle(line.important ? AnyShapeStyle(.tint) : AnyShapeStyle(.primary))
                            .lineLimit(family == .systemSmall ? 3 : 1)
                    }
                }
                if family == .systemSmall && entry.openCount > 1 {
                    Spacer(minLength: 0)
                    Text("+\(entry.openCount - 1) more")
                        .font(.system(.caption2, design: .monospaced))
                        .foregroundStyle(.tertiary)
                } else {
                    Spacer(minLength: 0)
                }
            }
        }
    }
}

struct NextStepsWidget: Widget {
    var body: some WidgetConfiguration {
        AppIntentConfiguration(kind: "ch.simk.tick.next",
                               intent: SelectProjectIntent.self,
                               provider: NextStepsProvider()) { entry in
            NextStepsWidgetView(entry: entry)
        }
        .configurationDisplayName("Next steps")
        .description("The next open steps of one project.")
        .supportedFamilies([.systemSmall, .systemMedium])
    }
}
