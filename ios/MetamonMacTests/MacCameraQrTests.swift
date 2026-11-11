import XCTest
import CoreImage
import AppKit
@testable import MetamonRenderer

/// Mac の撮影(#1043)と QR の読み取り(#1045)。カメラは偽物、QR は CoreImage で作った画像を Vision で読む。
@MainActor
final class MacCameraQrTests: XCTestCase {
    // MARK: - QR(Vision)

    private func qrImage(_ text: String) throws -> CGImage {
        let filter = try XCTUnwrap(CIFilter(name: "CIQRCodeGenerator"))
        filter.setValue(Data(text.utf8), forKey: "inputMessage")
        filter.setValue("M", forKey: "inputCorrectionLevel")
        // 小さすぎると読めないので拡大し、周りに白い余白を付ける
        let code = try XCTUnwrap(filter.outputImage).transformed(by: CGAffineTransform(scaleX: 10, y: 10))
        let canvas = CIImage(color: .white).cropped(to: code.extent.insetBy(dx: -40, dy: -40))
        let image = code.composited(over: canvas)
        return try XCTUnwrap(CIContext().createCGImage(image, from: image.extent))
    }

    func testDecodesGeneratedQrImage() throws {
        XCTAssertEqual(QrImageDecoder.decode(try qrImage("https://tailor-it.app/s/abc123")), "https://tailor-it.app/s/abc123")
    }

    func testDecodesQrFromPngDataAndFile() throws {
        let png = try XCTUnwrap(NSBitmapImageRep(cgImage: try qrImage("会員番号 0042")).representation(using: .png, properties: [:]))
        XCTAssertEqual(QrImageDecoder.decode(data: png), "会員番号 0042")
        let url = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString).appendingPathExtension("png")
        try png.write(to: url)
        defer { try? FileManager.default.removeItem(at: url) }
        XCTAssertEqual(QrImageDecoder.decode(fileAt: url), "会員番号 0042")
    }

    func testImageWithoutCodeReturnsNil() throws {
        let blank = CIImage(color: .white).cropped(to: CGRect(x: 0, y: 0, width: 200, height: 200))
        let cg = try XCTUnwrap(CIContext().createCGImage(blank, from: blank.extent))
        XCTAssertNil(QrImageDecoder.decode(cg))
        XCTAssertNil(QrImageDecoder.decode(data: Data("not an image".utf8)))
    }

    // MARK: - 撮る流れ(偽のカメラ)

    @MainActor
    private final class FakeDriver: MacCaptureDriving {
        var devices: [MacCaptureDevice] = [.init(id: "builtin", name: "内蔵"), .init(id: "iphone", name: "iPhone")]
        var onCode: ((String) -> Void)?
        var startError: Error?
        var started: [String?] = []
        var stops = 0
        var stopRecordings = 0
        var photo = NSImage(size: NSSize(width: 4, height: 3))
        var recordContinuation: CheckedContinuation<URL, Error>?

        func start(mode: MacCaptureMode, deviceID: String?) async throws {
            started.append(deviceID)
            if let startError { throw startError }
        }
        func stop() { stops += 1 }
        func capturePhoto() async throws -> PlatformImage { photo }
        func record(maxDuration: TimeInterval) async throws -> URL {
            try await withCheckedThrowingContinuation { recordContinuation = $0 }
        }
        func stopRecording() { stopRecordings += 1 }
    }

    func testPhotoFlowShootRetakeUse() async {
        let driver = FakeDriver()
        let flow = MacCaptureFlow(mode: .photo, driver: driver)
        XCTAssertEqual(flow.phase, .starting)
        await flow.start()
        XCTAssertEqual(flow.phase, .live)
        await flow.shutter()
        XCTAssertEqual(flow.phase, .reviewPhoto)
        XCTAssertNotNil(flow.photo)
        XCTAssertEqual(driver.stops, 1, "確かめている間はカメラを止める")
        await flow.retake()
        XCTAssertEqual(flow.phase, .live)
        XCTAssertNil(flow.photo)
        XCTAssertEqual(driver.started.count, 2)
    }

    func testShutterIgnoredUntilLive() async {
        let flow = MacCaptureFlow(mode: .photo, driver: FakeDriver())
        await flow.shutter()
        XCTAssertEqual(flow.phase, .starting)
        XCTAssertNil(flow.photo)
    }

    func testNoCameraAndDeniedAreUnavailable() async {
        let driver = FakeDriver()
        driver.startError = MacCaptureError.noCamera
        let flow = MacCaptureFlow(mode: .photo, driver: driver)
        await flow.start()
        XCTAssertEqual(flow.phase, .unavailable(.noCamera))
        driver.startError = MacCaptureError.denied
        await flow.start()
        XCTAssertEqual(flow.phase, .unavailable(.denied))
        driver.startError = MacCaptureError.failed
        await flow.start()
        XCTAssertEqual(flow.phase, .failed)
    }

    func testSwitchingDeviceRestartsWithThatCamera() async {
        let driver = FakeDriver()
        let flow = MacCaptureFlow(mode: .photo, driver: driver)
        await flow.start()
        await flow.select(device: "iphone")
        XCTAssertEqual(driver.started, [nil, "iphone"])
        XCTAssertEqual(flow.deviceID, "iphone")
        XCTAssertEqual(flow.phase, .live)
    }

    func testVideoFlowRecordStopAndRetakeDeletesFile() async throws {
        let driver = FakeDriver()
        let flow = MacCaptureFlow(mode: .video(maxDuration: 30), driver: driver)
        await flow.start()
        let recording = Task { await flow.shutter() }
        while driver.recordContinuation == nil { await Task.yield() }
        XCTAssertEqual(flow.phase, .recording)
        await flow.shutter()
        XCTAssertEqual(driver.stopRecordings, 1)
        let url = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString).appendingPathExtension("mov")
        try Data([0]).write(to: url)
        driver.recordContinuation?.resume(returning: url)
        await recording.value
        XCTAssertEqual(flow.phase, .reviewVideo)
        XCTAssertEqual(flow.video, url)
        await flow.retake()
        XCTAssertFalse(FileManager.default.fileExists(atPath: url.path), "撮り直した動画の一時ファイルは消す")
        XCTAssertEqual(flow.phase, .live)
    }

    func testUsedVideoIsHandedOverNotDeleted() async throws {
        let driver = FakeDriver()
        let flow = MacCaptureFlow(mode: .video(maxDuration: 30), driver: driver)
        await flow.start()
        let recording = Task { await flow.shutter() }
        while driver.recordContinuation == nil { await Task.yield() }
        let url = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString).appendingPathExtension("mov")
        try Data([0]).write(to: url)
        defer { try? FileManager.default.removeItem(at: url) }
        driver.recordContinuation?.resume(returning: url)
        await recording.value
        XCTAssertEqual(flow.takeVideo(), url)
        flow.cancel()
        XCTAssertTrue(FileManager.default.fileExists(atPath: url.path), "使う動画は取り込み側が消す")
    }

    func testQrFlowTakesFirstCodeOnly() async {
        let driver = FakeDriver()
        let flow = MacCaptureFlow(mode: .qr, driver: driver)
        await flow.start()
        driver.onCode?("first")
        driver.onCode?("second")
        XCTAssertEqual(flow.code, "first")
        XCTAssertEqual(driver.stops, 1)
    }
}
