import SwiftUI
import XCTest

/// #486: systemLarge の見た目を ImageRenderer で描く(落ちない・はみ出さないことの確認)。
/// `WIDGET_SHOTS_DIR` があれば PNG を書き出す(xcodebuild では `TEST_RUNNER_WIDGET_SHOTS_DIR=<dir>`)。
@MainActor
final class LargeWidgetRenderTests: XCTestCase {
    /// iPhone の systemLarge(6.1〜6.3 インチ)とホーム画面の余白。
    private let size = CGSize(width: 338, height: 354)
    private let padding: CGFloat = 16

    private var noTrend: WidgetSnapshot {
        var s = WidgetSnapshot.sample
        s.trend = nil
        return s
    }

    private var empty: WidgetSnapshot {
        var s = WidgetSnapshot.sample
        s.recent = []
        s.trend?.points = WidgetSnapshot.sampleTrendPoints(Array(repeating: 0, count: 7))
        s.trend?.total = "¥0"
        return s
    }

    private var statsOnly: WidgetSnapshot {
        WidgetSnapshot(appName: "BMI", stats: [
            .init(key: "a", label: "最新の BMI", value: "21.4"),
            .init(key: "b", label: "目標まで", value: "-1.2 kg"),
            .init(key: "c", label: "記録した日", value: nil),
        ], updatedAt: .now)
    }

    func testRendersEveryKind() throws {
        let cases: [(String, WidgetSnapshot, ColorScheme, DynamicTypeSize)] = [
            ("large-full-light", .sample, .light, .large),
            ("large-full-dark", .sample, .dark, .large),
            ("large-no-trend", noTrend, .light, .large),
            ("large-empty-records", empty, .dark, .large),
            ("large-stats-only", statsOnly, .light, .large),
            ("large-xxxl-type", .sample, .light, .xxxLarge),
            ("large-ax3-type", .sample, .dark, .accessibility3),
        ]
        for (name, snapshot, scheme, type) in cases {
            let image = try render(LargeStatsView(snapshot: snapshot), scheme: scheme, type: type)
            XCTAssertEqual(image.size, size, name)
            try save(image, name: name)
        }
        try save(try render(EmptyStateView(appName: nil), scheme: .light, type: .large), name: "large-no-app")
    }

    /// 中身の自然な高さがウィジェットの高さに収まる(はみ出すと下のフッターが切れる)。
    func testContentFitsAtDefaultAndLargeType() {
        for type in [DynamicTypeSize.large, .xxxLarge, .accessibility1, .accessibility3] {
            for snapshot in [WidgetSnapshot.sample, noTrend, empty, statsOnly] {
                let view = LargeStatsView(snapshot: snapshot)
                    .environment(\.dynamicTypeSize, type)
                    .frame(width: size.width - padding * 2)
                    .fixedSize(horizontal: false, vertical: true)
                let height = ImageRenderer(content: view).uiImage?.size.height ?? .infinity
                XCTAssertLessThanOrEqual(height, size.height - padding * 2, "\(snapshot.appName) \(type)")
            }
        }
    }

    private func render(_ content: some View, scheme: ColorScheme, type: DynamicTypeSize) throws -> UIImage {
        let view = content
            .padding(padding)
            .frame(width: size.width, height: size.height)
            .background { WidgetTheme.background }
            .clipShape(RoundedRectangle(cornerRadius: 22, style: .continuous))
            .environment(\.colorScheme, scheme)
            .environment(\.dynamicTypeSize, type)
        let renderer = ImageRenderer(content: view)
        renderer.scale = 3
        return try XCTUnwrap(renderer.uiImage)
    }

    private func save(_ image: UIImage, name: String) throws {
        guard let dir = ProcessInfo.processInfo.environment["WIDGET_SHOTS_DIR"], !dir.isEmpty else { return }
        let url = URL(fileURLWithPath: dir, isDirectory: true)
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        try XCTUnwrap(image.pngData()).write(to: url.appendingPathComponent("\(name).png"))
    }
}
