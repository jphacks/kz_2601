#if METAMON_STG
import XCTest
@testable import Metamon

/// テストだけのセクション(探す仕組みが別のバンドル・孫の subclass も拾うか)
final class DebugRegistryProbeSection: DebugMenuSection {
    override class var order: Int { 150 }
}

class DebugRegistryProbeParent: DebugMenuSection {}
final class DebugRegistryProbeChild: DebugRegistryProbeParent {}

/// #2079: 開発用メニューのセクションは 1 ファイル 1 subclass で、共有の一覧を書き換えずに並ぶ。
@MainActor
final class DebugMenuRegistryTests: XCTestCase {
    private func names(_ types: [DebugMenuSection.Type]) -> [String] {
        types.map { NSStringFromClass($0).components(separatedBy: ".").last ?? "" }
    }

    func testDiscoversAppSections() {
        let found = names(DebugMenuRegistry.discover())
        XCTAssertTrue(found.contains("DebugProSection"), "\(found)")
        XCTAssertTrue(found.contains("DebugExtCallSection"), "\(found)")
    }

    func testDiscoversSubclassesInOtherBundlesAndGrandchildren() {
        let found = names(DebugMenuRegistry.discover())
        XCTAssertTrue(found.contains("DebugRegistryProbeSection"), "\(found)")
        XCTAssertTrue(found.contains("DebugRegistryProbeParent"), "\(found)")
        XCTAssertTrue(found.contains("DebugRegistryProbeChild"), "\(found)")
        XCTAssertFalse(found.contains("DebugMenuSection"), "土台の型は並べない")
    }

    func testOrderedByOrderThenName() {
        let ordered = names(DebugMenuRegistry.ordered([DebugRegistryProbeChild.self, DebugExtCallSection.self,
                                                       DebugRegistryProbeSection.self, DebugProSection.self,
                                                       DebugRegistryProbeParent.self]))
        XCTAssertEqual(ordered, ["DebugProSection", "DebugRegistryProbeSection", "DebugExtCallSection",
                                 "DebugRegistryProbeChild", "DebugRegistryProbeParent"])
    }

    func testMenuListsAppSectionsInOrder() {
        let menu = names(DebugMenuRegistry.sections)
        let pro = menu.firstIndex(of: "DebugProSection")
        let ext = menu.firstIndex(of: "DebugExtCallSection")
        XCTAssertNotNil(pro)
        XCTAssertNotNil(ext)
        if let pro, let ext { XCTAssertLessThan(pro, ext) }
    }

    func testProSectionResetRestoresDefault() {
        let gate = ProGate.shared
        let saved = gate.limitDisabled
        defer { gate.limitDisabled = saved }
        gate.limitDisabled = !ProGate.defaultLimitDisabled
        DebugProSection.reset()
        XCTAssertEqual(gate.limitDisabled, ProGate.defaultLimitDisabled)
    }
}
#endif
