import XCTest
import UIKit
import MetamonDSL
@testable import MetamonRenderer

/// aiButton `inputPhoto`: いつもオンライン・写真を 1024px / 400 KB に縮めて依頼に付ける・写真だけでも材料になる。
final class AiButtonPhotoTests: XCTestCase {
    private func component(_ json: String) throws -> Component {
        try JSONDecoder().decode(Component.self, from: Data(json.utf8))
    }

    private let entity = try! JSONDecoder().decode(Entity.self, from: Data(
        #"{"id":"m","name":"薬","fields":[{"id":"rx","name":"処方箋","type":"photo"},{"id":"note","name":"メモ","type":"text"}]}"#.utf8))

    /// ざらついた模様(JPEG で縮みにくい)の画像。
    private func noisyImage(width: Int, height: Int) -> UIImage {
        let format = UIGraphicsImageRendererFormat.default()
        format.scale = 1
        return UIGraphicsImageRenderer(size: CGSize(width: width, height: height), format: format).image { ctx in
            var seed: UInt32 = 7
            for y in stride(from: 0, to: height, by: 4) {
                for x in stride(from: 0, to: width, by: 4) {
                    seed = seed &* 1_103_515_245 &+ 12345
                    UIColor(red: CGFloat(seed >> 24 & 0xff) / 255, green: CGFloat(seed >> 16 & 0xff) / 255,
                            blue: CGFloat(seed >> 8 & 0xff) / 255, alpha: 1).setFill()
                    ctx.fill(CGRect(x: x, y: y, width: 4, height: 4))
                }
            }
        }
    }

    private func pixelSize(ofBase64 b64: String) -> CGSize? {
        guard let data = Data(base64Encoded: b64), let img = UIImage(data: data) else { return nil }
        return CGSize(width: img.size.width * img.scale, height: img.size.height * img.scale)
    }

    func testPhotoAlwaysOnline() {
        XCTAssertEqual(AIGenerateRoute.choose(availability: .available, hostOnline: true, mock: false, photo: true), .online,
                       "端末内 AI があっても写真はオンライン")
        XCTAssertEqual(AIGenerateRoute.choose(availability: .available, hostOnline: false, mock: false, photo: true),
                       .unavailable(AIGenerateRoute.photoUnavailable))
    }

    func testDownscaleToLimits() throws {
        let b64 = try XCTUnwrap(AIPhotoInput.jpegBase64(from: noisyImage(width: 3000, height: 2000)))
        XCTAssertLessThanOrEqual(b64.count, AITextLimits.maxImageBase64)
        let size = try XCTUnwrap(pixelSize(ofBase64: b64))
        XCTAssertLessThanOrEqual(max(size.width, size.height), CGFloat(AITextLimits.maxImageSide))
        let jpeg = try XCTUnwrap(Data(base64Encoded: b64))
        XCTAssertEqual(Array(jpeg.prefix(3)), [0xFF, 0xD8, 0xFF], "JPEG")
    }

    func testSmallCapShrinksSide() throws {
        // 上限を小さくすると品質だけでは収まらず、辺も縮める
        let b64 = try XCTUnwrap(AIPhotoInput.jpegBase64(from: noisyImage(width: 1024, height: 1024), maxBase64: 60_000))
        XCTAssertLessThanOrEqual(b64.count, 60_000)
        let size = try XCTUnwrap(pixelSize(ofBase64: b64))
        XCTAssertLessThan(size.width, 1024)
    }

    func testOnlineRequestWithPhotoOnly() async throws {
        let c = try component(#"{"type":"aiButton","task":"extract","inputPhoto":"rx","output":"note","prompt":"薬の名前と飲み方"}"#)
        XCTAssertEqual(c.inputPhoto, "rx")
        let url = try XCTUnwrap(PhotoCodec.dataURL(from: noisyImage(width: 1600, height: 1200), maxSide: 1600, quality: 0.8))
        let values: [String: JSONValue] = ["rx": .string(url)]
        var r = try AIRequestBuilder.online(c, entity: entity, values: values, locale: "ja")
        XCTAssertNil(r.input, "写真だけ: 文字の材料は無し")
        try await AIPhotoInput.attach(c, values: values, to: &r)
        let image = try XCTUnwrap(r.image)
        XCTAssertEqual(image.mediaType, "image/jpeg")
        XCTAssertLessThanOrEqual(image.data.count, AITextLimits.maxImageBase64)
        let size = try XCTUnwrap(pixelSize(ofBase64: image.data))
        XCTAssertLessThanOrEqual(max(size.width, size.height), 1024)

        let body = try XCTUnwrap(String(data: JSONEncoder().encode(r), encoding: .utf8))
        XCTAssertTrue(body.contains(#""image":{"#) && body.contains(#""mediaType":"image\/jpeg""#), body.prefix(200).description)
        XCTAssertFalse(body.contains("data:image"), "data URL の接頭辞は送らない")
    }

    func testNoPhotoThrows() async throws {
        let c = try component(#"{"type":"aiButton","task":"extract","inputPhoto":"rx","output":"note"}"#)
        var r = try AIRequestBuilder.online(c, entity: entity, values: [:], locale: "ja")
        do {
            try await AIPhotoInput.attach(c, values: [:], to: &r)
            XCTFail("写真が無いのに付いた")
        } catch let e as AIPhotoInput.PhotoError {
            XCTAssertEqual(e, .noPhoto)
        }
        do {
            try await AIPhotoInput.attach(c, values: ["rx": .string("data:image/jpeg;base64,!!!")], to: &r)
            XCTFail("読めない写真が付いた")
        } catch let e as AIPhotoInput.PhotoError {
            XCTAssertEqual(e, .unreadable)
        }
        XCTAssertNil(r.image)
    }

    func testWithoutInputPhotoNothingAttached() async throws {
        let c = try component(#"{"type":"aiButton","task":"summarize","input":"note","output":"note"}"#)
        var r = try AIRequestBuilder.online(c, entity: entity, values: ["note": .string("本文")], locale: "ja")
        try await AIPhotoInput.attach(c, values: ["rx": .string("x")], to: &r)
        XCTAssertNil(r.image)
        XCTAssertFalse(try XCTUnwrap(String(data: JSONEncoder().encode(r), encoding: .utf8)).contains("image"))
    }

    // MARK: - https の写真(PR #720 のレビュー: 置き場を限る・読みながら上限で止める)

    func testOnlyStorageHostsAreAllowed() throws {
        let ok = "https://firebasestorage.googleapis.com/v0/b/x.appspot.com/o/users%2Fu%2Fapps%2Fa%2Fphotos%2F1.jpg?alt=media"
        XCTAssertTrue(AIPhotoInput.isAllowed(try XCTUnwrap(URL(string: ok))))
        XCTAssertTrue(AIPhotoInput.isAllowed(try XCTUnwrap(URL(string: "https://storage.googleapis.com/b/p.jpg"))))
        for bad in ["http://firebasestorage.googleapis.com/a.jpg", "https://evil.example.com/a.jpg",
                    "https://firebasestorage.googleapis.com.evil.example/a.jpg", "https://u:p@firebasestorage.googleapis.com/a.jpg",
                    "https://firebasestorage.googleapis.com:8443/a.jpg", "https://localhost/a.jpg", "https://169.254.169.254/latest"] {
            XCTAssertFalse(AIPhotoInput.isAllowed(try XCTUnwrap(URL(string: bad))), bad)
        }
    }

    func testOtherHostsAreRejectedBeforeLoading() async throws {
        let c = try component(#"{"type":"aiButton","task":"extract","inputPhoto":"rx","output":"note"}"#)
        var r = try AIRequestBuilder.online(c, entity: entity, values: [:], locale: "ja")
        do {
            try await AIPhotoInput.attach(c, values: ["rx": .string("https://evil.example.com/a.jpg")], to: &r)
            XCTFail("許可していない置き場の写真を読んだ")
        } catch let e as AIPhotoInput.PhotoError {
            XCTAssertEqual(e, .untrustedSource)
            XCTAssertNotNil(e.errorDescription)
        }
        XCTAssertNil(r.image)
    }

    private func stream(_ count: Int) -> AsyncStream<UInt8> {
        AsyncStream { cont in
            for _ in 0..<count { cont.yield(0xAB) }
            cont.finish()
        }
    }

    func testReadCappedStopsAtLimit() async throws {
        let data = try await AIPhotoInput.readCapped(stream(100), limit: 100)
        XCTAssertEqual(data.count, 100)
        do {
            _ = try await AIPhotoInput.readCapped(stream(101), limit: 100)
            XCTFail("上限を超えて読んだ")
        } catch let e as AIPhotoInput.PhotoError {
            XCTAssertEqual(e, .tooLarge)
        }
    }
}
