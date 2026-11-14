import Photos
import XCTest
#if canImport(UIKit)
import UIKit
#else
import AppKit
#endif
@testable import MetamonRenderer

/// 画像をカメラロールに保存する共通の口(#2220)。偽物で、結果の 3 つの分かれ道を確かめる。
@MainActor
final class PhotoLibrarySaverTests: XCTestCase {
    private func image() -> PlatformImage {
        let size = CGSize(width: 4, height: 4)
        #if canImport(UIKit)
        return UIGraphicsImageRenderer(size: size).image { ctx in
            ctx.cgContext.setFillColor(gray: 0, alpha: 1)
            ctx.cgContext.fill(CGRect(origin: .zero, size: size))
        }
        #else
        let img = NSImage(size: size)
        img.lockFocus()
        NSColor.black.setFill()
        NSRect(origin: .zero, size: size).fill()
        img.unlockFocus()
        return img
        #endif
    }

    // MARK: 結果の 3 つの分かれ道

    func testSavedWhenAuthorized() async {
        let library = FakePhotoLibrary(access: .authorized)
        let result = await PhotoLibrarySaver.save(image(), to: library)
        XCTAssertEqual(result, .saved)
        XCTAssertTrue(result.isSaved)
        XCTAssertEqual(library.authorizationCalls, 1)
        XCTAssertEqual(library.addCalls, 1)
    }

    /// 一部だけ許可でも「追加」はできる
    func testSavedWhenLimited() async {
        let library = FakePhotoLibrary(access: .limited)
        let result = await PhotoLibrarySaver.save(image(), to: library)
        XCTAssertEqual(result, .saved)
        XCTAssertEqual(library.addCalls, 1)
    }

    /// 許可が無い時は「失敗」ではなく「許可なし」(状態も分かる)。保存は試さない
    func testNotAuthorizedKeepsTheStatusAndDoesNotSave() async {
        for access in [PhotoAddAccess.notDetermined, .denied, .restricted] {
            let library = FakePhotoLibrary(access: access)
            let result = await PhotoLibrarySaver.save(image(), to: library)
            XCTAssertEqual(result, .notAuthorized(access), "\(access) で許可なしにならない")
            XCTAssertFalse(result.isSaved)
            XCTAssertEqual(library.addCalls, 0, "\(access) なのに保存を試した")
        }
    }

    /// 許可はあるのに保存できなかった時だけ「失敗」
    func testFailedWhenAddThrows() async {
        let library = FakePhotoLibrary(access: .authorized, addError: .changesFailed)
        let result = await PhotoLibrarySaver.save(image(), to: library)
        XCTAssertEqual(result, .failed)
        XCTAssertFalse(result.isSaved)
        XCTAssertEqual(library.addCalls, 1)
    }

    func testThreeResultsAreAllDifferent() async {
        let saved = await PhotoLibrarySaver.save(image(), to: FakePhotoLibrary(access: .authorized))
        let denied = await PhotoLibrarySaver.save(image(), to: FakePhotoLibrary(access: .denied))
        let failed = await PhotoLibrarySaver.save(image(), to: FakePhotoLibrary(access: .authorized, addError: .changesFailed))
        XCTAssertEqual(Set([String(describing: saved), String(describing: denied), String(describing: failed)]).count, 3)
        XCTAssertNotEqual(denied, failed, "許可なしと失敗が同じ扱いに戻っている")
    }

    // MARK: 許可の状態

    func testAccessMapsEveryPhotoKitStatus() {
        XCTAssertEqual(PhotoAddAccess(.notDetermined), .notDetermined)
        XCTAssertEqual(PhotoAddAccess(.denied), .denied)
        XCTAssertEqual(PhotoAddAccess(.restricted), .restricted)
        XCTAssertEqual(PhotoAddAccess(.limited), .limited)
        XCTAssertEqual(PhotoAddAccess(.authorized), .authorized)
    }

    func testOnlyAuthorizedAndLimitedCanAdd() {
        XCTAssertEqual(PhotoAddAccess.allCases.filter(\.canAdd), [.limited, .authorized])
    }

    // MARK: 差し替えられること

    func testLibraryIsReplaceable() async {
        let original = PhotoLibrarySaver.library
        defer { PhotoLibrarySaver.library = original }
        XCTAssertTrue(original is LivePhotoLibrary, "既定は PhotoKit の実体")
        let library = FakePhotoLibrary(access: .authorized)
        PhotoLibrarySaver.library = library
        let result = await PhotoLibrarySaver.save(image())
        XCTAssertEqual(result, .saved)
        XCTAssertEqual(library.addCalls, 1)
    }

    // MARK: QR の保存の文言(今までどおり: 許可なしも失敗も同じ)

    func testQrCodeSaveStateMapping() {
        XCTAssertEqual(QrCodeLeaf.SaveState(.saved), .saved)
        XCTAssertEqual(QrCodeLeaf.SaveState(.failed), .failed)
        for access in [PhotoAddAccess.notDetermined, .denied, .restricted] {
            XCTAssertEqual(QrCodeLeaf.SaveState(.notAuthorized(access)), .failed)
        }
    }
}
