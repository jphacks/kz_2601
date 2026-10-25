#if DEBUG
import Foundation
import MetamonDSL
import MetamonServices

/// 見た目監査(#131)用の DEBUG フック。実際に AI が生成した DSL をファイルから描画する。
/// - `METAMON_INITIAL_DSL_PATH=<path>`: DSL JSON(AppDSL 本文)を起動時の document にする。appId はファイル名ごとに分ける(ローカルキャッシュ混在防止)
/// - `METAMON_INITIAL_SCREEN=<screen>`: initialScreen を上書き(タブの 2 枚目を初期表示する等)
/// - `METAMON_SEED_DUMMY=<n>`: モック時、各 entity に field 型から n 件(1〜10、既定 4)のダミーレコードを作る
enum DebugDSLOverride {
    private static var env: [String: String] { ProcessInfo.processInfo.environment }

    static let document: AppDocument? = {
        guard let path = env["METAMON_INITIAL_DSL_PATH"], !path.isEmpty else { return nil }
        let url = URL(fileURLWithPath: path)
        do {
            var dsl = try AppDSL.load(contentsOf: url)
            if let screen = env["METAMON_INITIAL_SCREEN"], dsl.screen(screen) != nil { dsl.initialScreen = screen }
            print("[Metamon] DSL override: \(url.lastPathComponent) screens=\(dsl.screens.map(\.id))")
            return AppDocument(appId: "visual-" + url.deletingPathExtension().lastPathComponent, version: 1, dsl: dsl)
        } catch {
            print("[Metamon] DSL override failed: \(error)")
            return nil
        }
    }()

    /// ダミーレコード(override 未指定 or `METAMON_SEED_DUMMY` 無しなら nil)。
    static var seed: [String: [Record]]? {
        guard let dsl = document?.dsl, let raw = env["METAMON_SEED_DUMMY"] else { return nil }
        return DummyRecords.make(for: dsl, count: min(max(Int(raw) ?? 4, 1), 10))
    }
}

/// entity の field 型からそれらしいダミー値を作る。長い文字列・大きい金額を混ぜて崩れを炙り出す。
enum DummyRecords {
    static func make(for dsl: AppDSL, count: Int, now: Date = .now) -> [String: [Record]] {
        let iso = ISO8601DateFormatter()
        var out: [String: [Record]] = [:]
        for entity in dsl.entities {
            out[entity.id] = (0..<count).map { i in
                var data: [String: JSONValue] = [:]
                for f in entity.storedFields {
                    if let v = value(for: f, entity: entity, index: i, dsl: dsl, now: now) { data[f.id] = v }
                }
                // 新しい順に 1 時間ずつ前(`$createdAt` の並びと表示を確かめる・#250)
                let created = iso.string(from: now.addingTimeInterval(Double(-i) * 3600))
                var record = Record(id: "\(entity.id)-\(i + 1)", data: data, createdAt: created)
                if entity.isShared {
                    // 共有 entity: 偶数番は自分(モックの uid)、奇数番はほかの人の投稿
                    let mine = i % 2 == 0
                    record.author = mine ? MockAuthService.fixedUID : "dummy-member-\(i)"
                    record.authorName = mine ? (AuthorProfile.name ?? RecordSyncEngine.anonymousName) : otherNames[i % otherNames.count]
                }
                return record
            }
        }
        return out
    }

    private static let otherNames = ["さくら", "たけし", "匿名"]

    private static let shortWords = ["ランチ", "電車", "カフェ", "本", "日用品", "サークル費"]
    private static let longWords = ["駅前の新しくできたイタリアンレストランで友達と夕食", "Weekend grocery run at the big supermarket"]
    private static let amounts: [Double] = [980, 12_800, 450, 128_000, 3_200, 60]
    private static let integers: [Double] = [3, 12, 1, 150, 8, 42]
    private static let numbers: [Double] = [65.4, 170.2, 2.5, 1200.75, 0.8, 21.3]

    private static func value(for f: Field, entity: Entity, index i: Int, dsl: AppDSL, now: Date) -> JSONValue? {
        switch f.type {
        case .string:
            if f.id == entity.displayTitleField {
                return .string(i == 2 ? longWords[0] : "\(f.name) \(i + 1)")
            }
            return .string(shortWords[i % shortWords.count])
        case .text:
            return .string(i == 1 ? longWords[0] + "。" + longWords[1] : "\(f.name)のメモ \(i + 1)")
        case .currency:
            return .number(amounts[i % amounts.count])
        case .integer:
            return .number(integers[i % integers.count])
        case .number:
            return .number(numbers[i % numbers.count])
        case .boolean:
            return .bool(i % 2 == 0)
        case .date, .datetime:
            let d = Calendar.current.date(byAdding: .day, value: -i * 2, to: now) ?? now
            return .string(FieldCoding.encode(d, for: f))
        case .enum:
            guard let options = f.options, !options.isEmpty else { return nil }
            return .string(options[i % options.count])
        case .ref:
            guard let target = f.refEntity, dsl.entity(target) != nil else { return nil }
            return .string("\(target)-\(i % 3 + 1)")
        case .refs:
            guard let target = f.refEntity, dsl.entity(target) != nil else { return nil }
            return i % 3 == 0 ? .array([]) : .array([.string("\(target)-1"), .string("\(target)-\(i % 3 + 1)")])
        case .video:
            // 別の端末で撮った動画(この端末にファイルが無い)の見え方を確かめる(#206)
            return .string("video:dummy-\(i)")
        case .photo, .location:
            return nil
        }
    }
}
#endif
