import XCTest
import AppKit
@testable import Metamon

/// サイドバーの行の写し(#1281)の残し方と消し方。
@MainActor
final class MacAppThumbnailStoreTests: XCTestCase {
    private func makeStore() -> (MacAppThumbnailStore, URL) {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("thumbs-\(UUID().uuidString)", isDirectory: true)
        addTeardownBlock { try? FileManager.default.removeItem(at: dir) }
        return (MacAppThumbnailStore(directory: dir), dir)
    }

    private func sampleImage() -> NSImage {
        NSImage(size: CGSize(width: 40, height: 28), flipped: false) { rect in
            NSColor.systemIndigo.setFill()
            rect.fill()
            return true
        }
    }

    func testStoreWritesFileAndReadsBackFromDisk() async throws {
        let (store, dir) = makeStore()
        let before = store.revision
        store.store(sampleImage(), for: "app/1")
        XCTAssertGreaterThan(store.revision, before)
        XCTAssertNotNil(store.image(for: "app/1"))
        // 書き込みは裏で行う
        let file = store.url("app/1")
        XCTAssertEqual(file.lastPathComponent, "app_1.jpg")
        for _ in 0..<50 where !FileManager.default.fileExists(atPath: file.path) {
            try await Task.sleep(for: .milliseconds(20))
        }
        XCTAssertTrue(FileManager.default.fileExists(atPath: file.path))
        // 新しく作った置き場(起動し直し)からも読める
        XCTAssertNotNil(MacAppThumbnailStore(directory: dir).image(for: "app/1"))
    }

    func testRemoveForgetsImage() async throws {
        let (store, _) = makeStore()
        store.store(sampleImage(), for: "gone")
        let file = store.url("gone")
        for _ in 0..<50 where !FileManager.default.fileExists(atPath: file.path) {
            try await Task.sleep(for: .milliseconds(20))
        }
        store.remove(appId: "gone")
        XCTAssertNil(store.image(for: "gone"))
        XCTAssertFalse(FileManager.default.fileExists(atPath: file.path))
    }

    func testVoiceOnlyAppIsNotCaptured() {
        let (store, _) = makeStore()
        store.capture(appId: "voice", isVoiceOnly: true, anchor: MacThumbnailAnchor())
        XCTAssertNil(store.image(for: "voice"))
    }
}
