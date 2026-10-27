import Foundation

/// `chatThread`(#260)の並び・自分の判定・送るデータ。描画は Renderer の `ChatThreadLeaf`。
public enum ChatThread {
    /// 古い順(最新が末尾)。送信時刻は `$createdAt`。時刻の無い旧データは先頭、同時刻は id で安定させる。
    /// 未送信の自分のメッセージはローカルの createdAt(= 送った時刻)なので末尾に来る。
    public static func ordered(_ records: [Record]) -> [Record] {
        records
            .map { ($0, date($0.createdAt)) }
            .sorted { a, b in
                switch (a.1, b.1) {
                case let (x?, y?) where x != y: x < y
                case (nil, _?): true
                case (_?, nil): false
                default: a.0.id < b.0.id
                }
            }
            .map(\.0)
    }

    /// 自分のメッセージか(右に出す)。投稿者(`Record.author`)とサインイン中の uid を比べる。
    /// private の entity は全部自分。author の無いレコード(未送信の自分の分・旧データ)も自分扱い。uid が分からなければ他人の分は相手側。
    public static func isMine(_ record: Record, in entity: Entity?, uid: String?) -> Bool {
        guard entity?.isShared == true, let author = record.author else { return true }
        return author == uid
    }

    /// 相手の吹き出しの上に送信者名を出すか。続けて同じ人が送った 2 通目以降は省く(LINE と同じ)。
    public static func showsSender(_ record: Record, after previous: Record?, in entity: Entity?, uid: String?) -> Bool {
        guard entity?.isShared == true, !isMine(record, in: entity, uid: uid) else { return false }
        guard let previous else { return true }
        return isMine(previous, in: entity, uid: uid) || previous.author != record.author
    }

    /// 送るレコードの data。本文の前後の空白は落とし、本文も写真も無ければ nil(送れない)。
    /// `base` は entity の他の field の既定値(defaultValue)。
    public static func message(text: String, photo: String?, textField: String, photoField: String?,
                               base: [String: JSONValue] = [:]) -> [String: JSONValue]? {
        let body = text.trimmingCharacters(in: .whitespacesAndNewlines)
        let photo = photoField == nil ? nil : photo.flatMap { $0.isEmpty ? nil : $0 }
        guard !body.isEmpty || photo != nil else { return nil }
        var data = base
        if !body.isEmpty { data[textField] = .string(body) }
        if let photo, let photoField { data[photoField] = .string(photo) }
        return data
    }

    private static let fractional: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()

    /// RFC3339(小数秒あり / なし)→ Date。
    public static func date(_ s: String?) -> Date? {
        guard let s else { return nil }
        return fractional.date(from: s) ?? FieldCoding.datetimeFormatter.date(from: s)
    }
}
