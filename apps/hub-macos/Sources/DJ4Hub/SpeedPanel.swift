import SwiftUI

struct SpeedPanel: View {
    let title: String
    let values: [Double]
    let color: Color
    var body: some View {
        Panel(title: title, height: 168) {
            Text(values.last.map { ByteCountFormatter.string(fromByteCount: Int64($0), countStyle: .binary) + "/s" } ?? "—")
                .font(.system(size: 27, weight: .semibold, design: .rounded)).lineLimit(1).minimumScaleFactor(0.6)
            GeometryReader { geometry in
                if values.count >= 2 {
                    let peak = max(values.max() ?? 1, 1)
                    Path { path in
                        for (index, value) in values.enumerated() {
                            let point = CGPoint(x: geometry.size.width * CGFloat(index) / CGFloat(values.count - 1), y: geometry.size.height * (1 - value / peak))
                            if index == 0 { path.move(to: point) } else { path.addLine(to: point) }
                        }
                    }.stroke(color, style: StrokeStyle(lineWidth: 2, lineCap: .round, lineJoin: .round))
                } else { Text("等待流量采样").font(.system(size: 10)).foregroundStyle(.tertiary).frame(maxHeight: .infinity, alignment: .bottom) }
            }.frame(height: 44)
        }.frame(maxWidth: .infinity)
    }
}
