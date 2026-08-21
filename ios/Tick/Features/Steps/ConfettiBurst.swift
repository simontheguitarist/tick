import SwiftUI

/// A ~0.7s particle pop in the TUI's six hues — the phone-sized echo of the
/// terminal confetti. Pure Canvas, no timers to clean up; particles are frozen
/// at init and replayed against elapsed time.
struct ConfettiBurst: View {
    private struct Particle {
        let dx: Double   // pt/s
        let dy: Double
        let spin: Double // rad/s
        let size: Double
        let colorIndex: Int
        let round: Bool
    }

    private let particles: [Particle]
    private let start = Date()

    init(count: Int = 26) {
        var rng = SystemRandomNumberGenerator()
        particles = (0..<count).map { _ in
            Particle(
                dx: Double.random(in: -220...220, using: &rng),
                dy: Double.random(in: -260...40, using: &rng),
                spin: Double.random(in: -6...6, using: &rng),
                size: Double.random(in: 3...6, using: &rng),
                colorIndex: Int.random(in: 0..<Theme.confetti.count, using: &rng),
                round: Bool.random(using: &rng)
            )
        }
    }

    var body: some View {
        TimelineView(.animation) { timeline in
            Canvas { ctx, size in
                let t = timeline.date.timeIntervalSince(start)
                guard t < Theme.Motion.burst else { return }
                let progress = t / Theme.Motion.burst
                let origin = CGPoint(x: size.width * 0.3, y: size.height * 0.5)
                let gravity = 900.0
                for p in particles {
                    let x = origin.x + p.dx * t
                    let y = origin.y + p.dy * t + 0.5 * gravity * t * t
                    guard x > -10, x < size.width + 10, y < size.height + 10 else { continue }
                    let fade = progress > 0.6 ? 1 - (progress - 0.6) / 0.4 : 1
                    var rect = CGRect(x: x, y: y, width: p.size, height: p.size)
                    var c = ctx
                    c.opacity = fade
                    c.translateBy(x: rect.midX, y: rect.midY)
                    c.rotate(by: .radians(p.spin * t))
                    rect = CGRect(x: -p.size / 2, y: -p.size / 2, width: p.size, height: p.size * (p.round ? 1 : 0.6))
                    let path = p.round ? Path(ellipseIn: rect) : Path(roundedRect: rect, cornerRadius: 1)
                    c.fill(path, with: .color(Theme.confetti[p.colorIndex]))
                }
            }
        }
    }
}
