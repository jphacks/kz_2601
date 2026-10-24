import XCTest
import UIKit
@testable import MetamonRenderer

/// photoField の `source` 分岐とカメラ可否フォールバック、PhotoCodec の縮小仕様(長辺 ≤800px・JPEG data URL)。
final class PhotoFieldTests: XCTestCase {
    func testSourceModeParsing() {
        XCTAssertEqual(PhotoSourceMode(raw: "camera"), .camera)
        XCTAssertEqual(PhotoSourceMode(raw: "library"), .library)
        XCTAssertEqual(PhotoSourceMode(raw: "both"), .both)
        XCTAssertEqual(PhotoSourceMode(raw: "any"), .both, "schema 外の any は both 扱い")
        XCTAssertEqual(PhotoSourceMode(raw: nil), .both, "省略は both")
        XCTAssertEqual(PhotoSourceMode(raw: "hologram"), .both, "未知値でも落ちない")
    }

    func testPlanWhenCameraAvailable() {
        XCTAssertEqual(PhotoSourcePolicy.plan(for: .camera, cameraAvailable: true), .camera)
        XCTAssertEqual(PhotoSourcePolicy.plan(for: .library, cameraAvailable: true), .library)
        XCTAssertEqual(PhotoSourcePolicy.plan(for: .both, cameraAvailable: true), .choose)
    }

    func testPlanFallsBackToLibraryWithoutCamera() {
        for mode in [PhotoSourceMode.camera, .library, .both] {
            XCTAssertEqual(PhotoSourcePolicy.plan(for: mode, cameraAvailable: false), .library, "\(mode) はカメラ無しで写真選択へ")
        }
        XCTAssertEqual(PhotoSourcePolicy.plan(source: "camera", cameraAvailable: false), .library)
    }

    /// 実行環境の可否(iOS 26 シミュレータは仮想カメラ有り・iOS 17/18 シミュレータは無し)に応じて
    /// 家計簿の `source: camera` が camera か library のどちらかに必ず決まる(クラッシュせず未定義にならない)。
    @MainActor func testKakeiboReceiptPlanFollowsRuntimeAvailability() {
        let available = CameraPicker.isAvailable
        XCTAssertEqual(PhotoSourcePolicy.plan(source: "camera", cameraAvailable: available), available ? .camera : .library)
        XCTAssertEqual(PhotoSourcePolicy.plan(source: nil, cameraAvailable: available), available ? .choose : .library)
    }

    @MainActor func testDataURLShrinksToMaxSideAndIsJPEG() throws {
        let big = UIGraphicsImageRenderer(size: CGSize(width: 2400, height: 1600)).image { ctx in
            UIColor.systemOrange.setFill(); ctx.fill(CGRect(x: 0, y: 0, width: 2400, height: 1600))
        }
        let url = try XCTUnwrap(PhotoCodec.dataURL(from: big))
        XCTAssertTrue(url.hasPrefix("data:image/jpeg;base64,"))

        let decoded = try XCTUnwrap(PhotoCodec.image(from: .string(url)))
        let longSide = max(decoded.size.width * decoded.scale, decoded.size.height * decoded.scale)
        XCTAssertLessThanOrEqual(longSide, 800 + 1)
        XCTAssertEqual(decoded.size.width / decoded.size.height, 1.5, accuracy: 0.01, "アスペクト比維持")
        XCTAssertLessThan(url.utf8.count, 1_000_000, "Firestore 1MB 制限内")
    }

    @MainActor func testDataURLKeepsSmallImageSize() throws {
        let format = UIGraphicsImageRendererFormat.default()
        format.scale = 1   // 300x200 ピクセル(既定の 3x だと 900px になり縮小対象)
        let small = UIGraphicsImageRenderer(size: CGSize(width: 300, height: 200), format: format).image { ctx in
            UIColor.systemBlue.setFill(); ctx.fill(CGRect(x: 0, y: 0, width: 300, height: 200))
        }
        let data = try XCTUnwrap(small.pngData())
        let url = try XCTUnwrap(PhotoCodec.dataURL(from: data))
        let decoded = try XCTUnwrap(PhotoCodec.image(from: .string(url)))
        XCTAssertEqual(decoded.size.width * decoded.scale, 300, accuracy: 1, "800px 未満は拡大しない")
    }
}
