import Foundation

/// 変身一覧の整理(#277): 名前の変更・複製・並べ替えの純粋な部分(名前の検査・並べ替え後の順番)。
/// 名前と順番の正典はサーバー(`users/{uid}/apps`)。Apple ID に引き継いだ uid(#276)なら機種変更でも残る。
public enum AppListEdit {
    /// サーバー(appconfig `maxAppNameRunes`)と同じ上限。
    public static let maxNameLength = 40

    /// 実装の無いクライアント(テスト用など)が投げる。
    public struct Unsupported: LocalizedError {
        public var errorDescription: String? { "この接続先では使えません" }
    }
    public static let unsupported = Unsupported()

    /// 前後の空白を除いた名前。空 / 長すぎは nil。
    public static func normalizedName(_ raw: String) -> String? {
        let name = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !name.isEmpty, name.count <= maxNameLength else { return nil }
        return name
    }

    /// 複製の既定の名前。`suffixed`(「○○のコピー」等・呼び出し側で翻訳)が上限を超えるなら元の名前を後ろから削る。
    public static func copyName(of name: String, suffixed: (String) -> String) -> String {
        var base = name
        while suffixed(base).count > maxNameLength, !base.isEmpty { base.removeLast() }
        return suffixed(base)
    }

    /// `ids` の中で `id` を `index` の位置に動かした順番。`id` が無い / 範囲外は端に寄せる。
    public static func moved(_ ids: [String], id: String, to index: Int) -> [String] {
        guard let from = ids.firstIndex(of: id) else { return ids }
        var out = ids
        out.remove(at: from)
        out.insert(id, at: min(max(index, 0), out.count))
        return out
    }

    /// `ids`(先頭から)の順に `sortIndex` = 位置+1 を振る。`ids` に無いアプリはそのまま(サーバーの `PUT /v1/apps/order` と同じ)。
    public static func applyingOrder(_ ids: [String], to apps: [AppSummary]) -> [AppSummary] {
        let position = Dictionary(ids.enumerated().map { ($1, $0 + 1) }, uniquingKeysWith: { a, _ in a })
        return apps.map { app in
            var app = app
            if let p = position[app.id] { app.sortIndex = p }
            return app
        }
    }
}
