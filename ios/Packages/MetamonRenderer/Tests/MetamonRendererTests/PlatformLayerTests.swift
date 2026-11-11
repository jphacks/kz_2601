import XCTest
@testable import MetamonRenderer

/// 置き換え層(#1051): `PlatformGraphics` を通る縮小で上下が逆にならない・長い辺が 800px になる・QR の画像を Vision で読める(macOS)。
/// 写真は iOS と macOS の両方で回す。
final class PlatformLayerTests: XCTestCase {
    /// 画素(RGBA)を読む。行 0 が画像の上。
    private func pixels(_ cg: CGImage) throws -> (w: Int, h: Int, at: (Int, Int) -> (r: UInt8, g: UInt8, b: UInt8)) {
        let w = cg.width, h = cg.height
        var buf = [UInt8](repeating: 0, count: w * h * 4)
        let ctx = try XCTUnwrap(CGContext(data: &buf, width: w, height: h, bitsPerComponent: 8, bytesPerRow: w * 4,
                                          space: CGColorSpace(name: CGColorSpace.sRGB)!,
                                          bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue))
        ctx.draw(cg, in: CGRect(x: 0, y: 0, width: w, height: h))
        let copy = buf
        return (w, h, { x, y in let i = (y * w + x) * 4; return (copy[i], copy[i + 1], copy[i + 2]) })
    }

    /// 上半分が赤・下半分が青の縦長の写真(原点は左上)
    private func halfRedPhoto(scale: CGFloat) -> PlatformImage {
        testImage(width: 1200, height: 1800, scale: scale, opaque: true) { ctx in
            PlatformColor.red.setFill(); ctx.fill(CGRect(x: 0, y: 0, width: 1200, height: 900))
            PlatformColor.blue.setFill(); ctx.fill(CGRect(x: 0, y: 900, width: 1200, height: 900))
        }
    }

    private func assertUprightAndShrunk(_ url: String, file: StaticString = #filePath, line: UInt = #line) throws {
        let decoded = try XCTUnwrap(PhotoCodec.image(from: .string(url)), file: file, line: line)
        let cg = try XCTUnwrap(decoded.cgImage, file: file, line: line)
        XCTAssertEqual(cg.height, 800, "長い辺は 800px", file: file, line: line)
        XCTAssertEqual(cg.width, 533, "短い辺は比率どおり(切り捨て)", file: file, line: line)
        let px = try pixels(cg)
        let top = px.at(px.w / 2, px.h / 8), bottom = px.at(px.w / 2, px.h * 7 / 8)
        XCTAssertGreaterThan(top.r, 200, "上が赤のまま: \(top)", file: file, line: line)
        XCTAssertLessThan(top.b, 60, "上が赤のまま: \(top)", file: file, line: line)
        XCTAssertGreaterThan(bottom.b, 200, "下が青のまま: \(bottom)", file: file, line: line)
        XCTAssertLessThan(bottom.r, 60, "下が青のまま: \(bottom)", file: file, line: line)
    }

    @MainActor func testPhotoCodecKeepsOrientationAndShrinksTo800() throws {
        // 画面倍率 3x(iPhone のカメラの画像と同じ)と 1x(取り込んだファイル)
        for scale in [CGFloat(3), 1] {
            try assertUprightAndShrunk(try XCTUnwrap(PhotoCodec.dataURL(from: halfRedPhoto(scale: scale))))
        }
    }

    @MainActor func testPhotoCodecFromEncodedDataKeepsOrientation() throws {
        let png = try XCTUnwrap(halfRedPhoto(scale: 1).pngData())
        try assertUprightAndShrunk(try XCTUnwrap(PhotoCodec.dataURL(from: png)))
    }

    // MARK: - QR(Vision・macOS)
    // iOS は QRCodeTests が CIDetector で確かめる(iOS シミュレータの Vision はバーコードを返さないため Mac だけ)。

    #if os(macOS)
    func testGeneratedQrImageDecodesWithVision() throws {
        for text in ["https://example.com/members?id=00123", "会員番号 00123 / 山田 花子"] {
            let img = try XCTUnwrap(QRCodeImage.image(for: text))
            XCTAssertEqual(QrImageDecoder.decode(try XCTUnwrap(img.cgImage)), text)
        }
    }

    /// 書き出した PNG(取り込み・ドラッグで来る形)からも読める
    func testQrImageDecoderReadsExportedPng() throws {
        let img = try XCTUnwrap(QRCodeImage.image(for: "MEMBER:00123"))
        let png = try XCTUnwrap(img.pngData())
        XCTAssertEqual(QrImageDecoder.decode(data: png), "MEMBER:00123")
        let url = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString).appendingPathExtension("png")
        try png.write(to: url)
        defer { try? FileManager.default.removeItem(at: url) }
        XCTAssertEqual(QrImageDecoder.decode(fileAt: url), "MEMBER:00123")
        XCTAssertNil(QrImageDecoder.decode(data: Data("not an image".utf8)))
    }
    #endif
}
