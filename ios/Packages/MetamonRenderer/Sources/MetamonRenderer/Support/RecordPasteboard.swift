#if os(iOS)
import UIKit
#elseif os(macOS)
import AppKit
#endif
import UniformTypeIdentifiers
import MetamonDSL

/// 記録のコピー・貼り付け(#1871)のクリップボードの読み書き。中身の作り方と対応は `RecordClipboard`(MetamonDSL)。
/// 独自の型(`app.tailorit.record`)と文字(「名前: 値」の行)の 2 つを 1 項目に載せ、10 分で消える。
/// iOS は `UIPasteboard` の失効と `.localOnly`(この端末の中だけ)。Mac(#2251)は `NSPasteboard` に失効が無いので、
/// 失効の時刻を別の型(`expiryType`)で一緒に載せ、読む時に過ぎていれば無いものとして扱う。
@MainActor
public enum RecordPasteboard {
    /// 載せるもの。載せる項目が無い記録は nil(メニューを出さない)。文字は画面と同じ書き方。
    public static func payload(record: Record, entity: Entity, rt: any DSLRuntime) -> RecordClipboard.Payload? {
        RecordClipboard.make(appId: rt.appId, appName: rt.dsl.app.name, entity: entity, data: record.data)
    }

    /// 文字で貼る先(メモ・メール)向けの「名前: 値」の行(画面と同じ書き方)
    public static func text(_ payload: RecordClipboard.Payload, entity: Entity, rt: any DSLRuntime) -> String {
        RecordClipboard.plainText(payload) { f in rt.display(f.value, field: entity.field(f.id)) }
    }

    #if os(iOS)
    /// クリップボードに書く。書けなければ false。
    @discardableResult
    public static func write(_ payload: RecordClipboard.Payload, text: String, to board: UIPasteboard = .general,
                             now: Date = Date()) -> Bool {
        guard let data = RecordClipboard.encode(payload) else { return false }
        var item: [String: Any] = [RecordClipboard.typeIdentifier: data]
        if !text.isEmpty { item[UTType.utf8PlainText.identifier] = text }
        board.setItems([item], options: [.expirationDate: now.addingTimeInterval(RecordClipboard.expiry), .localOnly: true])
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
    #elseif os(macOS)
    /// 記録の型
    public static let recordType = NSPasteboard.PasteboardType(RecordClipboard.typeIdentifier)
    /// 失効の時刻(1970 年からの秒・文字)を載せる型。Mac だけ(`NSPasteboard` は失効を持たない)
    public static let expiryType = NSPasteboard.PasteboardType(RecordClipboard.typeIdentifier + ".expires")

    /// クリップボードに書く(記録・失効の時刻・文字)。書けなければ false。
    @discardableResult
    public static func write(_ payload: RecordClipboard.Payload, text: String, to board: NSPasteboard = .general,
                             now: Date = Date()) -> Bool {
        guard let data = RecordClipboard.encode(payload) else { return false }
        let item = NSPasteboardItem()
        item.setData(data, forType: recordType)
        item.setString(expiryStamp(now: now), forType: expiryType)
        if !text.isEmpty { item.setString(text, forType: .string) }
        board.clearContents()
        return board.writeObjects([item])
    }

    /// 記録が載っていて、まだ失効していないか。
    public static func hasRecord(on board: NSPasteboard = .general, now: Date = Date()) -> Bool {
        board.availableType(from: [recordType]) != nil && !isExpired(board.string(forType: expiryType), now: now)
    }

    /// 載っている記録を読む。無い・失効した・壊れている時は nil。
    public static func read(from board: NSPasteboard = .general, now: Date = Date()) -> RecordClipboard.Payload? {
        guard !isExpired(board.string(forType: expiryType), now: now),
              let data = board.data(forType: recordType) else { return nil }
        return RecordClipboard.decode(data)
    }

    /// ⌘C・ドラッグ(SwiftUI の `onCopyCommand`・`onDrag`)で渡す中身。記録・失効の時刻・文字を載せる
    public static func itemProvider(_ payload: RecordClipboard.Payload, text: String, now: Date = Date()) -> NSItemProvider {
        let provider = text.isEmpty ? NSItemProvider() : NSItemProvider(object: text as NSString)
        if let data = RecordClipboard.encode(payload) {
            provider.registerDataRepresentation(forTypeIdentifier: RecordClipboard.typeIdentifier, visibility: .ownProcess) { done in
                done(data, nil)
                return nil
            }
            let stamp = Data(expiryStamp(now: now).utf8)
            provider.registerDataRepresentation(forTypeIdentifier: expiryType.rawValue, visibility: .ownProcess) { done in
                done(stamp, nil)
                return nil
            }
        }
        return provider
    }

    /// 行のドラッグ(#2252): 項目つきの記録と文字。サイドバーの別のアプリに落とすと値の入ったフォームが開く。
    /// 載せる項目が無い記録は文字だけ(`text` が空なら何も載せない)
    static func dragItem(record: Record, entity: Entity?, rt: any DSLRuntime, text: String) -> NSItemProvider {
        if let entity, let payload = payload(record: record, entity: entity, rt: rt) {
            return itemProvider(payload, text: text.isEmpty ? self.text(payload, entity: entity, rt: rt) : text)
        }
        return text.isEmpty ? NSItemProvider() : NSItemProvider(object: text as NSString)
    }

    /// 失効の時刻の書き方(1970 年からの秒)
    static func expiryStamp(now: Date) -> String {
        String(now.addingTimeInterval(RecordClipboard.expiry).timeIntervalSince1970)
    }

    /// 失効の時刻が無い(別のアプリが書いた)・読めない・過ぎた時は失効とみなす
    static func isExpired(_ stamp: String?, now: Date) -> Bool {
        guard let stamp, let at = Double(stamp) else { return true }
        return now.timeIntervalSince1970 >= at
    }
    #endif

    // MARK: - 画面から

    /// 「記録をコピー」: 記録を載せて、ホストに一言出させる。
    public static func copy(record: Record, entity: Entity, rt: any DSLRuntime) {
        guard let p = payload(record: record, entity: entity, rt: rt) else { return }
        let ok = write(p, text: text(p, entity: entity, rt: rt))
        rt.recordClipboard?.notifyRecordClipboard(ok ? L("記録をコピーしました") : L("記録をコピーできませんでした"))
    }

    /// このフォームに貼れるか: 記録が載っていて、変えられるフォームで、貼れる項目がある
    public static func canPaste(into form: FormDraft, hasRecord: Bool) -> Bool {
        hasRecord && !form.isReadOnly && !RecordClipboard.copyableFields(of: form.entity).isEmpty
    }

    /// 「記録を貼り付け」: 合う項目をフォームに入れ(保存はしない)、入った・合わなかった項目を一言で出す。
    public static func paste(into form: FormDraft, rt: any DSLRuntime) {
        guard let p = read() else {
            rt.recordClipboard?.notifyRecordClipboard(unreadableMessage(hasRecord: hasRecord()))
            return
        }
        apply(p, into: form, rt: rt)
    }

    /// 読めた記録をフォームに入れて一言出す(貼り付け・Mac のドラッグ)。
    public static func apply(_ payload: RecordClipboard.Payload, into form: FormDraft, rt: any DSLRuntime) {
        let result = RecordClipboard.paste(payload, into: form.entity, appId: rt.appId, isAdmin: rt.viewerIsAdmin == true)
        for (k, v) in result.values { form.values[k] = v }
        rt.recordClipboard?.notifyRecordClipboard(message(for: result))
    }

    /// 読めなかった時の一言。記録は載っているのに読めない時は、貼り付けを許可されなかった(または壊れていた)。
    static func unreadableMessage(hasRecord: Bool) -> String {
        hasRecord ? L("貼り付けが許可されませんでした") : L("貼り付けられる記録がありません")
    }

    /// 結果の一言: 「2 項目を入れました: 題名、著者」+ 合わなかった項目の名前。
    public static func message(for result: RecordClipboard.PasteResult) -> String {
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
