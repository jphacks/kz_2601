import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer

/// 適合テスト(#352): `docs/dsl/conformance/cases/<名前>/case.json` → 描画の木 → 同じフォルダの `expected.json` と比べる。
/// 期待値の更新は iOS だけ: `CONFORMANCE_UPDATE=1`(xcodebuild では `TEST_RUNNER_CONFORMANCE_UPDATE=1`)で expected.json を書き直す。
/// Android は expected.json を書き換えずに合わせる。形と規則は `docs/dsl/conformance/README.md`。
final class ConformanceTests: XCTestCase {
    private static let repoRoot: URL = {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return url
    }()
    private static let conformanceDir = repoRoot.appending(path: "docs/dsl/conformance")
    private static let casesDir = conformanceDir.appending(path: "cases")
    private static var updating: Bool { ProcessInfo.processInfo.environment["CONFORMANCE_UPDATE"] == "1" }

    /// 評価器のキャッシュは runtime の ObjectIdentifier で引くので、テスト中は runtime を解放しない(番地の再利用で別ケースの値を拾わない)
    @MainActor private static var keepAlive: [ConformanceRuntime] = []

    private static func caseNames() throws -> [String] {
        try FileManager.default.contentsOfDirectory(atPath: casesDir.path)
            .filter { FileManager.default.fileExists(atPath: casesDir.appending(path: "\($0)/case.json").path) }
            .sorted()
    }

    @MainActor func testAllCasesMatchExpectedTrees() throws {
        let names = try Self.caseNames()
        XCTAssertGreaterThanOrEqual(names.count, 10, "cases が読めていない")
        var failures: [String] = []
        for name in names {
            let dir = Self.casesDir.appending(path: name)
            let actual = try Self.tree(forCase: dir)
            let expectedURL = dir.appending(path: "expected.json")
            if Self.updating {
                try Self.encode(actual).write(to: expectedURL)
                continue
            }
            guard let data = try? Data(contentsOf: expectedURL) else {
                failures.append("\(name): expected.json がありません(iOS で CONFORMANCE_UPDATE=1 を付けて作る)")
                continue
            }
            let expected = try JSONDecoder().decode(JSONValue.self, from: data)
            let diffs = Self.diff(expected, actual, path: "$")
            if !diffs.isEmpty {
                let shown = diffs.prefix(30).joined(separator: "\n  ")
                failures.append("\(name): \(diffs.count) 件の差分\n  \(shown)\(diffs.count > 30 ? "\n  …" : "")")
            }
        }
        if Self.updating { print("[conformance] \(names.count) 件の expected.json を書き直しました") }
        XCTAssert(failures.isEmpty, "描画の木が expected.json と違う(意図した変更なら CONFORMANCE_UPDATE=1 で更新して差分をレビュー):\n" + failures.joined(separator: "\n"))
    }

    /// 出てくる部品の type の集合 + coverage.json の notYetCovered = スキーマの全 type(どちらにも無い・両方にある、で赤)。
    @MainActor func testCoverageListIsUpToDate() throws {
        var covered = Set<String>()
        for name in try Self.caseNames() {
            let spec = try CaseSpec.load(Self.casesDir.appending(path: name), docsDir: Self.repoRoot.appending(path: "docs/dsl"))
            for s in spec.dsl.screens { for c in s.allComponents { covered.insert(c.rawType) } }
        }
        let schema = try Self.schemaComponentTypes()
        let coverage = try JSONDecoder().decode([String: [String]].self, from: Data(contentsOf: Self.conformanceDir.appending(path: "coverage.json")))
        let notYet = Set(coverage["notYetCovered"] ?? [])
        XCTAssertEqual(schema.subtracting(covered).subtracting(notYet), [], "どの case にも無いのに coverage.json の notYetCovered にも無い type")
        XCTAssertEqual(notYet.intersection(covered), [], "case で使われたので coverage.json の notYetCovered から外す type")
        XCTAssertEqual(notYet.subtracting(schema), [], "スキーマに無い type が notYetCovered にある")
    }

    /// 差分の読みやすさ(壊れた時に場所と値が出る)。
    func testDiffReportsPathAndValues() {
        let a: JSONValue = .object(["nodes": .array([.object(["value": "¥1,200"])])])
        let b: JSONValue = .object(["nodes": .array([.object(["value": "¥1,300"])])])
        XCTAssertEqual(Self.diff(a, b, path: "$"), [#"$.nodes[0].value: expected "¥1,200" but got "¥1,300""#])
        XCTAssertEqual(Self.diff(.array([1, 2]), .array([1]), path: "$"), ["$: array length expected 2 but got 1", "$[1]: missing (expected 2)"])
    }

    // MARK: - ケース → 木

    @MainActor static func tree(forCase dir: URL) throws -> JSONValue {
        let spec = try CaseSpec.load(dir, docsDir: repoRoot.appending(path: "docs/dsl"))
        let saved = (tz: NSTimeZone.default, lang: LanguagePreference.shared.setting, clock: DSLClock.fixed)
        NSTimeZone.default = spec.timeZone
        LanguagePreference.shared.setting = spec.language
        DSLClock.fixed = spec.now
        defer {
            NSTimeZone.default = saved.tz
            LanguagePreference.shared.setting = saved.lang
            DSLClock.fixed = saved.clock
        }

        let rt = ConformanceRuntime(dsl: spec.dsl, store: spec.records, currentUserId: spec.userId)
        keepAlive.append(rt)
        let tree = RenderTree(runtime: rt, components: spec.components)

        // 画面ごと(DSL の順)。タブの根はタブのスタック、それ以外は main に新規で開いた状態
        let screens = spec.dsl.screens.map { s -> JSONValue in
            let route = Route(screen: s.id, stack: spec.dsl.app.hasTabs && spec.dsl.app.tabs.contains { $0.screen == s.id } ? Route.tabStack(s.id) : "main")
            let (form, locals) = rt.session(for: route)
            return tree.screen(route, form: form, locals: locals)
        }

        var steps: [JSONValue] = []
        for op in spec.operations {
            rt.effects = []
            let ok = rt.apply(op, tree: tree)
            let top = rt.top
            let (form, locals) = rt.session(for: top)
            var step: [String: JSONValue] = ["op": op, "top": RenderTree.route(top), "effects": .array(rt.effects)]
            if !ok { step["error"] = "no visible target" }
            step["screen"] = tree.screen(top, form: form, locals: locals)
            steps.append(.object(step))
        }
        var out: [String: JSONValue] = ["screens": .array(screens)]
        if !steps.isEmpty { out["steps"] = .array(steps) }
        return .object(out)
    }

    static func encode(_ v: JSONValue) throws -> Data {
        let enc = JSONEncoder()
        enc.outputFormatting = [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes]
        return try enc.encode(v) + Data("\n".utf8)
    }

    // MARK: - 差分

    static func diff(_ e: JSONValue, _ a: JSONValue, path: String) -> [String] {
        switch (e, a) {
        case let (.object(eo), .object(ao)):
            var out: [String] = []
            for k in Set(eo.keys).union(ao.keys).sorted() {
                let p = "\(path).\(k)"
                switch (eo[k], ao[k]) {
                case let (ev?, av?): out += diff(ev, av, path: p)
                case let (ev?, nil): out.append("\(p): missing (expected \(short(ev)))")
                case let (nil, av?): out.append("\(p): unexpected \(short(av))")
                default: break
                }
            }
            return out
        case let (.array(ea), .array(aa)):
            var out: [String] = ea.count == aa.count ? [] : ["\(path): array length expected \(ea.count) but got \(aa.count)"]
            for i in 0..<max(ea.count, aa.count) {
                let p = "\(path)[\(i)]"
                if i < ea.count, i < aa.count { out += diff(ea[i], aa[i], path: p) }
                else if i < ea.count { out.append("\(p): missing (expected \(short(ea[i])))") }
                else { out.append("\(p): unexpected \(short(aa[i]))") }
            }
            return out
        default:
            return e == a ? [] : ["\(path): expected \(short(e)) but got \(short(a))"]
        }
    }

    private static func short(_ v: JSONValue) -> String {
        let enc = JSONEncoder()
        enc.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
        let s = (try? enc.encode(v)).flatMap { String(data: $0, encoding: .utf8) } ?? "?"
        return s.count > 160 ? String(s.prefix(160)) + "…" : s
    }

    // MARK: - スキーマ(ComponentRegistryTests と同じ読み方)

    private static func schemaComponentTypes() throws -> Set<String> {
        let data = try Data(contentsOf: repoRoot.appending(path: "docs/dsl/dsl.schema.json"))
        let defs = try XCTUnwrap((try JSONSerialization.jsonObject(with: data) as? [String: Any])?["$defs"] as? [String: Any])
        var types = Set(try XCTUnwrap(((defs["Container"] as? [String: Any])?["properties"] as? [String: Any])?["type"] as? [String: Any])["enum"] as? [String] ?? [])
        for branch in try XCTUnwrap((defs["Leaf"] as? [String: Any])?["anyOf"] as? [[String: Any]]) {
            let name = String(try XCTUnwrap(branch["$ref"] as? String).split(separator: "/").last!)
            let def = try XCTUnwrap(defs[name] as? [String: Any])
            types.insert(try XCTUnwrap(((def["properties"] as? [String: Any])?["type"] as? [String: Any])?["const"] as? String))
        }
        return types
    }
}

// MARK: - case.json

/// `{dsl, records, device, operations}`。dsl は中身そのもの、または `docs/dsl` からの相対パス(例: "examples/kakeibo.json")。
struct CaseSpec {
    var dsl: AppDSL
    var records: [String: [Record]]
    var now: Date
    var timeZone: TimeZone
    var language: AppLanguage
    var userId: String?
    var components: Set<String>?
    var operations: [JSONValue]

    static let defaultNow = "2026-11-12T12:00:00+09:00"

    static func load(_ dir: URL, docsDir: URL) throws -> CaseSpec {
        let raw = try JSONDecoder().decode(JSONValue.self, from: Data(contentsOf: dir.appending(path: "case.json")))
        guard case let .object(o) = raw else { throw CocoaError(.coderReadCorrupt) }
        let dslData: Data
        if let path = o["dsl"]?.stringValue {
            dslData = try Data(contentsOf: docsDir.appending(path: path))
        } else {
            dslData = try JSONEncoder().encode(o["dsl"] ?? .null)
        }
        let device = o["device"]?.objectValue ?? [:]
        let iso = ISO8601DateFormatter()
        let caps = device["capabilities"]?.objectValue
        var components = caps?["components"]?.arrayValue.map { Set($0.compactMap(\.stringValue)) }
        if let missing = caps?["missingComponents"]?.arrayValue {
            components = (components ?? ComponentRegistry.standard.registeredTypes).subtracting(missing.compactMap(\.stringValue))
        }
        let locale = device["locale"]?.stringValue ?? "ja_JP"
        return CaseSpec(
            dsl: try JSONDecoder().decode(AppDSL.self, from: dslData),
            records: try JSONDecoder().decode([String: [Record]].self, from: JSONEncoder().encode(o["records"] ?? .object([:]))),
            now: try XCTUnwrap(iso.date(from: device["now"]?.stringValue ?? defaultNow), "device.now は ISO 8601"),
            timeZone: TimeZone(identifier: device["timeZone"]?.stringValue ?? "Asia/Tokyo") ?? .current,
            language: locale.hasPrefix("en") ? .en : .ja,
            userId: device["userId"]?.stringValue,
            components: components,
            operations: o["operations"]?.arrayValue ?? []
        )
    }
}

// MARK: - 操作を受けるランタイム

/// レコードとナビゲーションを持つテスト用ランタイム。アクションの実行はアプリ本体の `AppRuntime.perform` と同じ規則を写す
/// (遷移は同じ `NavigationResolver`)。`AppRuntime` はアプリのターゲットにあってパッケージのテストから呼べないため。
@MainActor
@Observable
final class ConformanceRuntime: DSLRuntime {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab: String
    var loading: Set<String> = []
    var store: [String: [Record]]
    var currentUserId: String?
    var sheet: Route?
    var fullScreen: Route?
    var effects: [JSONValue] = []
    private var sessions: [String: (FormDraft?, ScreenLocals)] = [:]
    private var nextId = 1

    init(dsl: AppDSL, store: [String: [Record]], currentUserId: String?) {
        self.dsl = dsl
        self.store = store
        self.currentUserId = currentUserId
        selectedTab = dsl.app.hasTabs ? dsl.initialTab : ""
    }

    func records(for entity: String) -> [Record] { store[entity] ?? [] }
    func load(entities: Set<String>, force: Bool) async {}
    func close(_ route: Route) { nav = NavigationResolver.close(nav, route) }
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant(paths[stack] ?? []) }
    func run(_ action: Action, _ ctx: ActionContext) async { perform(action, ctx) }

    private var nav: NavigationState {
        get { NavigationState(paths: paths, selectedTab: selectedTab, sheet: sheet, fullScreen: fullScreen) }
        set { paths = newValue.paths; selectedTab = newValue.selectedTab; sheet = newValue.sheet; fullScreen = newValue.fullScreen }
    }

    /// いま一番上に見えている画面(全画面 > シート > 選択中のタブ / main)。
    var top: Route {
        if let full = fullScreen { return paths["full"]?.last ?? full }
        if let sheet { return paths["sheet"]?.last ?? sheet }
        let stack = dsl.app.hasTabs ? Route.tabStack(selectedTab) : "main"
        return paths[stack]?.last ?? (dsl.app.hasTabs ? Route(screen: selectedTab, stack: stack) : Route(screen: dsl.initialScreen))
    }

    /// 画面ごとのフォームのドラフトと UI 状態(`ScreenView` の @State に当たる)。
    func session(for route: Route) -> (FormDraft?, ScreenLocals) {
        if let s = sessions[route.id] { return s }
        var form: FormDraft?
        if let screen = dsl.screen(route.screen), let eid = screen.entity, let entity = dsl.entity(eid) {
            let record = route.recordId.flatMap { id in records(for: eid).first { $0.id == id } }
            form = ScreenView.makeDraft(entity: entity, record: record, rt: self)
        }
        let s = (form, ScreenLocals())
        sessions[route.id] = s
        return s
    }

    /// 操作 1 つ。対象が無い・見えていない(hidden / unavailable など)なら false。
    func apply(_ op: JSONValue, tree: RenderTree) -> Bool {
        guard case let .object(o) = op else { return false }
        let route = top
        let (form, locals) = session(for: route)
        let screen = dsl.screen(route.screen)
        let ctx = ActionContext(route: route, form: form)
        if let id = o["tap"]?.stringValue {
            guard let c = screen?.allComponents.first(where: { $0.id == id }), let a = c.action,
                  tree.state(of: c, ctx: RenderContext(route: route, form: form, locals: locals)) == "rendered" else { return false }
            dispatch(a, ctx)
        } else if let label = o["tapToolbar"]?.stringValue {
            guard let tb = screen?.toolbar.first(where: { $0.label == label }) else { return false }
            dispatch(tb.action, ctx)
        } else if let row = o["tapRow"]?.objectValue {
            // component 省略時は画面で最初の onItemTap を持つ部品(id の無い list 用)
            let id = row["component"]?.stringValue
            guard let c = screen?.allComponents.first(where: { id == nil ? $0.onItemTap != nil : $0.id == id }), let a = c.onItemTap,
                  tree.state(of: c, ctx: RenderContext(route: route, form: form, locals: locals)) == "rendered" else { return false }
            let rows = ListLeaf.rows(c, rt: self, locals: locals)
            let i = Int(row["index"]?.doubleValue ?? 0)
            guard rows.indices.contains(i) else { return false }
            dispatch(a, ActionContext(route: route, form: form, recordId: rows[i].id))
        } else if let input = o["input"]?.objectValue {
            guard let form, let f = input["field"]?.stringValue else { return false }
            form.values[f] = input["value"] ?? .null
        } else if let search = o["search"]?.objectValue {
            guard let key = search["field"]?.stringValue else { return false }
            locals.values[key] = search["text"]?.stringValue ?? ""
        } else if o["back"] != nil {
            close(route)
        } else {
            return false
        }
        return true
    }

    private func dispatch(_ action: Action, _ ctx: ActionContext) {
        if let message = action.confirmMessage { effects.append(.object(["confirm": .string(message)])) } // 確認は「はい」で進める
        perform(action, ctx)
    }

    private func navigate(to screen: String, presentation: Presentation?, recordId: String?, from route: Route) {
        nav = NavigationResolver.navigate(nav, to: screen, presentation: presentation, recordId: recordId, from: route, dsl: dsl)
    }

    private func perform(_ action: Action, _ ctx: ActionContext) {
        switch action.type {
        case .navigate:
            guard let screen = action.screen else { return }
            if let ref = action.prefillRef {
                guard let parentId = ctx.currentRecordId else {
                    effects.append(.object(["rejected": "unsavedParent"]))
                    return
                }
                FormPrefill.set([ref: .string(parentId)])
                navigate(to: screen, presentation: action.presentation, recordId: nil, from: ctx.route)
                // 新規フォームは開いた時に初期値を受け取る
                _ = session(for: top)
                return
            }
            navigate(to: screen, presentation: action.presentation, recordId: ctx.recordId, from: ctx.route)
        case .back:
            close(ctx.route)
        case .saveRecord:
            guard let form = ctx.form, let entity = action.entity ?? ctx.form?.entity.id else {
                effects.append(.object(["rejected": "noForm"]))
                return
            }
            let missing = form.missingRequired
            guard missing.isEmpty else {
                effects.append(.object(["rejected": .object(["missing": .array(missing.map(JSONValue.string))])]))
                return
            }
            let payload = form.payload
            if let id = form.recordId, let i = store[entity]?.firstIndex(where: { $0.id == id }) {
                store[entity]?[i].data = payload
                effects.append(.object(["saved": .object(["entity": .string(entity), "recordId": .string(id), "data": .object(payload)])]))
            } else {
                let id = "new-\(nextId)"
                nextId += 1
                store[entity, default: []].append(Record(id: id, data: payload))
                effects.append(.object(["saved": .object(["entity": .string(entity), "recordId": .string(id), "data": .object(payload)])]))
            }
            sessions = sessions.filter { $0.value.0 !== form } // 保存した画面のドラフトは捨てる(開き直すと新規)
            if let screen = action.screen {
                navigate(to: screen, presentation: action.presentation, recordId: nil, from: ctx.route)
            } else {
                close(ctx.route)
            }
        case .deleteRecord:
            guard let entity = action.entity ?? ctx.form?.entity.id, let id = ctx.recordId ?? ctx.form?.recordId else { return }
            store[entity]?.removeAll { $0.id == id }
            effects.append(.object(["deleted": .object(["entity": .string(entity), "recordId": .string(id)])]))
            if ctx.recordId == nil { close(ctx.route) }
        default:
            // OS の機能を呼ぶもの(URL・通知・書き出し・ライブアクティビティ・音声)は「何を頼んだか」だけ残す
            effects.append(.object(["request": RenderTree.action(action)]))
        }
    }
}
