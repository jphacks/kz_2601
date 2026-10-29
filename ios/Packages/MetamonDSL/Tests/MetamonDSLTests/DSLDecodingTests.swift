import XCTest
@testable import MetamonDSL

/// docs/dsl/examples/*.json(= backend の CI でも検証される正典 4 本)が Swift 側でもデコードできることを保証する。
final class DSLDecodingTests: XCTestCase {
    private func load(_ name: String) throws -> AppDSL { try ExamplesFixture.load(name) }

    func testExampleNamesMatchRepository() throws {
        let onDisk = try FileManager.default.contentsOfDirectory(atPath: ExamplesFixture.examplesDir.path)
            .filter { $0.hasSuffix(".json") }
            .map { String($0.dropLast(5)) }
            .sorted()
        XCTAssertEqual(onDisk, DSLExamples.names.sorted(), "DSLExamples.names と docs/dsl/examples がずれている")
    }

    func testAllExamplesDecode() throws {
        for name in DSLExamples.names {
            let dsl = try load(name)
            XCTAssertEqual(dsl.schemaVersion, 1, name)
            XCTAssertNotNil(dsl.screen(dsl.initialScreen), "\(name): initialScreen must exist")
            for s in dsl.screens {
                for c in s.allComponents {
                    XCTAssertNotEqual(c.type, .unknown, "\(name)/\(s.id): unknown component \(c.rawType)")
                }
                for tb in s.toolbar {
                    XCTAssertNotEqual(tb.action.type, .unknown, "\(name)/\(s.id): unknown toolbar action")
                }
            }
        }
    }

    func testInitialIsVoiceOnly() throws {
        let dsl = try load("initial")
        XCTAssertTrue(dsl.isVoiceOnly)
        XCTAssertEqual(dsl.screens.count, 1)
        XCTAssertEqual(dsl.screens[0].allComponents.filter { $0.type == .voiceInput }.count, 1)
        XCTAssertTrue(dsl.screens[0].isVoiceScreen)
    }

    func testKakeiboStructure() throws {
        let dsl = try load("kakeibo")
        XCTAssertEqual(dsl.app.voiceEntry, .floating)
        XCTAssertEqual(dsl.entities.first?.id, "expense")
        XCTAssertEqual(dsl.entities.first?.field("amount")?.type, .currency)
        let form = try XCTUnwrap(dsl.screen("expense_form"))
        XCTAssertEqual(form.entity, "expense")
        XCTAssertEqual(form.toolbar.first?.action.type, .saveRecord)
        XCTAssertTrue(form.allComponents.contains { $0.type == .datePicker })
        XCTAssertEqual(form.referencedEntities, ["expense"])
        let home = try XCTUnwrap(dsl.screen("home"))
        XCTAssertTrue(home.toolbar.isEmpty, "追加は右下の button だけ(#416)")
        XCTAssertEqual(home.allComponents.first { $0.type == .button }?.action?.presentation, .sheet)
        XCTAssertTrue(home.allComponents.contains { $0.type == .list && $0.onItemTap?.type == .navigate })
    }

    func testTrainingAndWalkNativeFeatures() throws {
        let training = try load("training")
        XCTAssertEqual(training.app.notifications.first?.repeat, .daily)
        XCTAssertEqual(training.app.notifications.first?.time, "08:00")
        let walk = try load("walk")
        let form = try XCTUnwrap(walk.screen("memo_form"))
        XCTAssertTrue(form.allComponents.contains { $0.type == .photoField && $0.source == "camera" })
        XCTAssertTrue(form.allComponents.contains { $0.action?.type == .scheduleNotification && $0.action?.notification?.repeat == .once })
    }

    func testUnknownComponentAndEnumAreLenient() throws {
        let json = #"{"id":"s","title":"t","components":[{"type":"vstack","children":[{"type":"text","text":"a","style":"neon"},{"type":"hologram"},"garbage"]},{"type":"button","label":"x","action":{"type":"teleport"}}]}"#
        let screen = try JSONDecoder().decode(Screen.self, from: Data(json.utf8))
        XCTAssertEqual(screen.components.count, 2)
        XCTAssertEqual(screen.components[0].children.count, 2) // "garbage" は skip
        XCTAssertEqual(screen.components[0].children[1].type, .unknown)
        XCTAssertEqual(screen.components[0].children[1].rawType, "hologram")
        XCTAssertEqual(screen.components[1].action?.type, .unknown)
        XCTAssertEqual(screen.components[1].action?.rawType, "teleport")
    }

    func testTabs() throws {
        let kakeibo = try load("kakeibo")
        XCTAssertTrue(kakeibo.app.hasTabs)
        XCTAssertEqual(kakeibo.app.tabs.map(\.screen), ["home", "stats"])
        XCTAssertEqual(kakeibo.initialTab, "home")
        XCTAssertFalse(try load("walk").app.hasTabs)
        XCTAssertFalse(try load("initial").app.hasTabs)
        XCTAssertEqual(try load("initial").initialTab, "")
    }

    func testEnvelopeDecodes() throws {
        let json = #"{"appId":"a1","version":7,"dsl":{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"home","entities":[],"screens":[{"id":"home","title":"h","components":[]}]}}"#
        let doc = try AppDocument.decode(Data(json.utf8))
        XCTAssertEqual(doc.version, 7)
        XCTAssertEqual(doc.appId, "a1")
        XCTAssertEqual(doc.dsl.app.voiceEntry, .none)
    }

    func testFallbackDocumentIsUsable() {
        XCTAssertEqual(AppDocument.fallback.dsl.initialScreen, "home")
        XCTAssertNotNil(AppDocument.fallback.dsl.screen("home"))
    }

    /// 表示系ギャラリー(gallery-display.json): 11 種が全部デコードされ、props が落ちていないこと。
    func testGalleryDisplayDecodes() throws {
        let dsl = try load("gallery-display")
        XCTAssertEqual(dsl.app.tabs.map(\.screen), ["overview", "history"])
        let all = dsl.screens.flatMap(\.allComponents)
        let expected: Set<ComponentType> = [.label, .icon, .progressBar, .gauge, .badge, .link, .markdown, .avatar, .table, .timeline, .emptyState]
        XCTAssertEqual(expected.subtracting(Set(all.map(\.type))), [], "ギャラリーに無い表示系 type")

        let overview = try XCTUnwrap(dsl.screen("overview"))
        let budget = try XCTUnwrap(overview.components.first { $0.id == "budget" })
        XCTAssertEqual(budget.type, .progressBar)
        XCTAssertEqual(budget.target, 30000)
        XCTAssertEqual(budget.entity, "expense")
        XCTAssertEqual(budget.filter?.op, "thisMonth")
        let fixed = try XCTUnwrap(overview.components.first { $0.id == "fixed" })
        XCTAssertEqual(fixed.value, 0.6)
        let gauge = try XCTUnwrap(overview.components.first { $0.id == "spent" })
        XCTAssertEqual(gauge.min, 0)
        XCTAssertEqual(gauge.max, 30000)
        XCTAssertEqual(overview.components.first { $0.id == "me" }?.name, "Tailor It")
        XCTAssertEqual(overview.components.first { $0.id == "official" }?.url, "https://jphacks.com")
        XCTAssertTrue(overview.components.first { $0.id == "tags" }?.children.contains { $0.type == .badge && $0.color == "red" } == true)

        let history = try XCTUnwrap(dsl.screen("history"))
        let table = try XCTUnwrap(history.components.first { $0.id == "recent_table" })
        XCTAssertEqual(table.columns?.map(\.field), ["date", "memo", "amount"])
        XCTAssertEqual(table.columns?.first?.title, "日付")
        XCTAssertEqual(table.limit, 10)
        let feed = try XCTUnwrap(history.components.first { $0.id == "feed" })
        XCTAssertEqual(feed.dateField, "date")
        XCTAssertEqual(feed.titleField, "memo")
        XCTAssertEqual(feed.subtitleField, "category")
        XCTAssertEqual(history.components.first { $0.id == "empty" }?.message, "レシートを撮ると、ここに並びます")
        XCTAssertEqual(overview.referencedEntities, ["expense"])
    }

    /// 親から子を作る(#256): navigate.prefillRef を読み、無い navigate は nil のまま。
    func testPrefillRefDecodes() throws {
        let dsl = try load("post_comments")
        let form = try XCTUnwrap(dsl.screen("post_form"))
        let button = try XCTUnwrap(form.allComponents.first { $0.id == "write_comment" })
        XCTAssertEqual(button.action?.type, .navigate)
        XCTAssertEqual(button.action?.prefillRef, "post")
        XCTAssertEqual(button.action?.screen, "comment_form")
        XCTAssertNil(dsl.screen("home")?.toolbar.first?.action.prefillRef)
    }
}
