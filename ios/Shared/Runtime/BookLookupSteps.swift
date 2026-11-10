import Foundation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// `lookupBook`: フォームの ISBN から本の情報を引き、書名などの field を上書きする(保存はしない)。
/// 端末は appconfig にだけ聞く(`POST /v1/lookup/book`)。見つからない・つながらない時はメッセージだけで手入力に戻る。
/// UIKit に依存しないので iOS と Mac で共有する(#978)。
@MainActor
enum BookLookupSteps {
    /// 引いてフォームに入れる。戻り値は toast に出す一言(待つ間にフォームが変わって何もしなかった時は nil)。
    static func run(_ action: Action, form: FormDraft?, client: (any BookLookupClient)?) async -> String? {
        guard let spec = action.bookLookup, let form, !form.isReadOnly else {
            return L10n.tr("本の情報はフォームの画面で読み込めます")
        }
        let raw = (form.values[spec.isbnField] ?? .null).displayString
        guard !raw.trimmingCharacters(in: .whitespaces).isEmpty else {
            return L10n.tr("先に ISBN を入力してください")
        }
        guard let isbn = BookLookupPlan.normalizeISBN(raw) else {
            return L10n.tr("ISBN を確かめてください(10 桁か 13 桁)")
        }
        guard let client else {
            return L10n.tr("本の情報を読み込めません。手で入力してください")
        }
        // 待つ間に別の記録へ移った・ISBN を打ち直した・欄を手で直した時に、古い結果で上書きしない
        let recordId = form.recordId
        let before = form.values
        do {
            let book = try await client.lookup(isbn: isbn)
            guard !form.isReadOnly, form.recordId == recordId,
                  (form.values[spec.isbnField] ?? .null) == (before[spec.isbnField] ?? .null) else { return nil }
            let values = BookLookupPlan.values(spec, book: book, entity: form.entity)
            for (id, v) in values where (form.values[id] ?? .null) == (before[id] ?? .null) { form.values[id] = v }
            form.values[spec.isbnField] = .string(isbn)
            return L10n.tr("「\(book.title)」の情報を入れました(出典: \(book.source.name))")
        } catch let e as APIError where e.status == 404 {
            return L10n.tr("この ISBN の本は見つかりませんでした。手で入力してください")
        } catch let e as APIError where e.isBookLookupOff {
            return L10n.tr("本の情報の取得は今使えません。手で入力してください")
        } catch let e as APIError where e.status == 400 {
            return L10n.tr("ISBN を確かめてください(10 桁か 13 桁)")
        } catch {
            return L10n.tr("本の情報を読み込めません。手で入力してください")
        }
    }
}
