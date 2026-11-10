#if os(macOS)
import XCTest
import AppKit
import SwiftUI
import PencilKit
@testable import MetamonRenderer

/// #1342: ⌘Z / ⇧⌘Z はキャンバスにフォーカスがある時だけキャンバスが受ける(SwiftUI の中に置いても)。⌥⌘Z はメニューへ通す。
@MainActor
final class MacDrawingKeyTests: XCTestCase {
    private struct Host: NSViewRepresentable {
        let view: MacDrawingView
        func makeNSView(context: Context) -> MacDrawingView { view }
        func updateNSView(_ v: MacDrawingView, context: Context) {}
    }

    private func key(_ chars: String, _ flags: NSEvent.ModifierFlags, in window: NSWindow) throws -> NSEvent {
        try XCTUnwrap(NSEvent.keyEvent(with: .keyDown, location: .zero, modifierFlags: flags, timestamp: 0,
                                       windowNumber: window.windowNumber, context: nil, characters: chars,
                                       charactersIgnoringModifiers: chars, isARepeat: false, keyCode: 6))
    }

    func testCommandZGoesToFocusedCanvas() throws {
        let session = DrawingSession(value: .null)
        let handle = CanvasHandle()
        let canvas = MacDrawingView(session: session, handle: handle)
        let window = NSWindow(contentRect: CGRect(x: 0, y: 0, width: 300, height: 300), styleMask: [.titled],
                              backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false
        let host = NSHostingView(rootView: Host(view: canvas).frame(width: 300, height: 300))
        window.contentView = host
        host.layoutSubtreeIfNeeded()
        XCTAssertTrue(canvas.isDescendant(of: host))
        handle.attach(canvas)

        var b = DrawingStrokeBuilder()
        b.add(CGPoint(x: 10, y: 10), time: 0)
        b.add(CGPoint(x: 100, y: 10), time: 0.1)
        let undo = try XCTUnwrap(canvas.undoManager)
        undo.groupsByEvent = false
        undo.beginUndoGrouping()
        session.add(try XCTUnwrap(b.stroke()), undo: undo)
        undo.endUndoGrouping()
        XCTAssertEqual(session.drawing.strokes.count, 1)

        // キャンバスにフォーカスが無ければメニューへ通す
        window.makeFirstResponder(nil)
        XCTAssertFalse(window.performKeyEquivalent(with: try key("z", .command, in: window)))
        XCTAssertEqual(session.drawing.strokes.count, 1)

        XCTAssertTrue(window.makeFirstResponder(canvas))
        XCTAssertFalse(window.performKeyEquivalent(with: try key("z", [.command, .option], in: window)), "⌥⌘Z は版の取り消し")
        XCTAssertTrue(window.performKeyEquivalent(with: try key("z", .command, in: window)))
        XCTAssertEqual(session.drawing.strokes.count, 0, "⌘Z で戻す")
        XCTAssertTrue(handle.canRedo)
        XCTAssertTrue(window.performKeyEquivalent(with: try key("Z", [.command, .shift], in: window)))
        XCTAssertEqual(session.drawing.strokes.count, 1, "⇧⌘Z でやり直す")
        XCTAssertTrue(handle.canUndo)
        window.close()
    }
}
#endif
