#if DEBUG && METAMON_STG
import MetamonRenderer
import SwiftUI

/// 開発用メニュー: 到着までの時間と Look Around の口(#2230)を部品なしで試す。DEBUG の版だけ。
final class DebugMapExtrasSection: DebugMenuSection {
    override class var order: Int { 270 }
    override class var title: String { "到着時間と Look Around" }
    override class var footer: String? {
        "今の場所から決まった 3 か所(金沢駅・兼六園・21 世紀美術館)への徒歩の時間と、Look Around があるかを求めます。位置情報の許可が要ります。"
    }
    @MainActor override class func makeBody() -> AnyView { AnyView(DebugMapExtrasRows()) }
}

private struct DebugMapExtrasRows: View {
    @State private var result: String?
    @State private var running = false

    private static let places: [(String, MapPoint)] = [
        ("金沢駅", MapPoint(latitude: 36.5781, longitude: 136.6480)),
        ("兼六園", MapPoint(latitude: 36.5621, longitude: 136.6625)),
        ("21 世紀美術館", MapPoint(latitude: 36.5609, longitude: 136.6582)),
    ]

    var body: some View {
        Button {
            Task { await run() }
        } label: {
            DebugKnobLabel(title: "3 か所の徒歩の時間を求める", effect: .immediate)
        }
        .disabled(running)
        .accessibilityIdentifier("debugMenu.mapExtras.run")
        if let result {
            Text(verbatim: result)
                .font(.footnote.monospaced())
                .textSelection(.enabled)
                .accessibilityIdentifier("debugMenu.mapExtras.result")
        }
    }

    private func run() async {
        running = true
        defer { running = false }
        let etas = await MapExtras.etas(to: Self.places.map(\.1), transport: .walking)
        var lines: [String] = []
        for (i, place) in Self.places.enumerated() {
            let scene = await MapExtras.lookAroundScene(at: place.1)
            let time = etas[i].map { "\(Int(($0.travelTime / 60).rounded())) 分 \(Int($0.distance)) m" } ?? "nil"
            lines.append("\(place.0): \(time) lookAround=\(scene != nil)")
        }
        result = lines.joined(separator: "\n")
    }
}
#endif
