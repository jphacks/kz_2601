#if os(iOS)
import UIKit
import UniformTypeIdentifiers
import MetamonDSL

/// 記録のコピー・貼り付け(#1871)のクリップボードの読み書き。中身の作り方と対応は `RecordClipboard`(MetamonDSL)。
/// 独自の型(`app.tailorit.record`)と文字(「名前: 値」の行)の 2 つを 1 項目に載せ、10 分で消える。
/// `UIPasteboard.general` はユニバーサルクリップボードで同じ Apple アカウントの Mac にも届く(Mac 側の貼り付けは後で作る)。
@MainActor
public enum RecordPasteboard {
    /// 載せるもの。載せる項目が無い記録は nil(メニューを出さない)。文字は画面と同じ書き方。
    public static func payload(record: Record, entity: Entity, rt: any DSLRuntime) -> RecordClipboard.Payload? {
        RecordClipboard.make(appId: rt.appId, appName: rt.dsl.app.name, entity: entity, data: record.data)
    }

    /// クリップボードに書く。書けなければ false。
    @discardableResult
    public static func write(_ payload: RecordClipboard.Payload, text: String, to board: UIPasteboard = .general,
                             now: Date = Date()) -> Bool {
        guard let data = RecordClipboard.encode(payload) else { return false }
        var item: [String: Any] = [RecordClipboard.typeIdentifier: data]
        if !text.isEmpty { item[UTType.utf8PlainText.identifier] = text }
        board.setItems([item], options: [.expirationDate: now.addingTimeInterval(RecordClipboard.expiry)])
        return true
    }

    /// 記録が載っているか(中身は読まないので、貼り付けの許可は聞かれない)。
    public static func hasRecord(on board: UIPasteboard = .general) -> Bool {
        board.contains(pasteboardTypes: [RecordClipboard.typeIdentifier])
    }

    /// 載っている記録を読む。無い・壊れている時は nil。
    public static func read(from board: UIPasteboard = .general) -> RecordClipboard.Payload? {
        guard let data = board.data(forPasteboardType: RecordClipboard.typeIdentifier) else { return nil }
        return RecordClipboard.decode(data)
    }

    // MARK: - 画面から

    /// 「記録をコピー」: 記録を載せて、ホストに一言出させる。
    static func copy(record: Record, entity: Entity, rt: any DSLRuntime) {
        guard let p = payload(record: record, entity: entity, rt: rt) else { return }
        let text = RecordClipboard.plainText(p) { f in rt.display(f.value, field: entity.field(f.id)) }
        let ok = write(p, text: text)
        rt.recordClipboard?.notifyRecordClipboard(ok ? L("記録をコピーしました") : L("記録をコピーできませんでした"))
    }

    /// 「記録を貼り付け」: 合う項目をフォームに入れ(保存はしない)、入った・合わなかった項目を一言で出す。
    static func paste(into form: FormDraft, rt: any DSLRuntime) {
        guard let p = read() else {
            rt.recordClipboard?.notifyRecordClipboard(L("貼り付けられる記録がありません"))
            return
        }
        let result = RecordClipboard.paste(p, into: form.entity)
        for (k, v) in result.values { form.values[k] = v }
        rt.recordClipboard?.notifyRecordClipboard(message(for: result))
    }

    /// 結果の一言: 「2 項目を入れました: 題名、著者」+ 合わなかった項目の名前。
    static func message(for result: RecordClipboard.PasteResult) -> String {
        let sep = L("・")
        var lines: [String] = []
        if result.matched.isEmpty {
            lines.append(L("このフォームに合う項目はありませんでした"))
        } else {
            lines.append(L("\(result.matched.count) 項目を入れました: \(result.matched.joined(separator: sep))"))
        }
        if !result.unmatched.isEmpty {
            lines.append(L("合わなかった項目: \(result.unmatched.joined(separator: sep))"))
        }
        return lines.joined(separator: "\n")
    }
}
#endif
