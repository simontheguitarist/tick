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
        ((try? StoreFile.appGroup().read())??.liveProjects ?? []).map { ProjectEntity(id: $0.id, name: $0.name) }
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
        guard let doc = (try? StoreFile.appGroup().read()) ?? nil else {
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

    // The TUI palette (Theme.swift lives in the app target; the widget
    // compiles only Shared/, so the few hues it needs are repeated here).
    private let cyan = Color(red: 0.16, green: 0.80, blue: 1.00)
    private let yellow = Color(red: 0.99, green: 1.00, blue: 0.42)
    private let green = Color(red: 0.47, green: 1.00, blue: 0.27)
    private let amber = Color(red: 1.00, green: 0.72, blue: 0.42)
    private let pink = Color(red: 1.00, green: 0.44, blue: 0.55)
    private let ink = Color(red: 0.95, green: 0.95, blue: 0.97)
    private let dim = Color(red: 0.37, green: 0.39, blue: 0.47)
    private let secondary = Color(red: 0.60, green: 0.63, blue: 0.71)

    var body: some View {
        Group {
            if let name = entry.projectName {
                content(name)
            } else {
                VStack(spacing: 6) {
                    Text("✓")
                        .font(.system(.title3, design: .monospaced).weight(.bold))
                        .foregroundStyle(green)
                    Text("open tick to get started")
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(secondary)
                        .multilineTextAlignment(.center)
                }
            }
        }
        .containerBackground(for: .widget) { Color("WidgetBackground") }
        .widgetURL(entry.projectID.flatMap { URL(string: "tick://project/\($0)") })
    }

    private func content(_ name: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline, spacing: 4) {
                Text("›").foregroundStyle(dim)
                Text(name)
                    .foregroundStyle(cyan)
                    .lineLimit(1)
                Spacer(minLength: 4)
                Text(entry.openCount == 0 ? "—" : "\(entry.openCount)")
                    .foregroundStyle(entry.openCount == 0 ? dim : yellow)
            }
            .font(.system(.subheadline, design: .monospaced).weight(.semibold))
            if entry.steps.isEmpty {
                Spacer()
                HStack(spacing: 6) {
                    Text("✓").foregroundStyle(green)
                    Text("all clear").foregroundStyle(secondary)
                }
                .font(.system(.caption, design: .monospaced))
                Spacer()
            } else {
                let shown = family == .systemSmall ? Array(entry.steps.prefix(1)) : entry.steps
                ForEach(shown) { line in
                    HStack(alignment: .firstTextBaseline, spacing: 6) {
                        Text(line.important ? "!" : "\(line.number).")
                            .font(.system(.caption, design: .monospaced).weight(.bold))
                            .foregroundStyle(line.important ? pink : dim)
                        Text(line.text)
                            .font(family == .systemSmall ? .subheadline : .caption)
                            .fontWeight(line.important ? .semibold : .regular)
                            .foregroundStyle(line.important ? amber : ink)
                            .lineLimit(family == .systemSmall ? 3 : 1)
                    }
                }
                if family == .systemSmall && entry.openCount > 1 {
                    Spacer(minLength: 0)
                    Text("+\(entry.openCount - 1) more")
                        .font(.system(.caption2, design: .monospaced))
                        .foregroundStyle(dim)
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
