import SwiftUI
import UniformTypeIdentifiers
import MetamonDSL

/// `fileImport`: ボタンでファイルを選び、entity に記録を足す。
/// - csv: 1 行 = 1 件(列の当て方・型の変換は `RecordImport`)。足す前に「n 件を追加・m 行は読めない」を確かめる。
/// - text / pdf: 本文を取り出して `field` に入れた記録を 1 件(PDF の画像だけのページは端末内の文字認識)。
/// ファイルはシステムのファイル選択(別プロセス・権限の確認なし)から読むだけで、送らない・コピーしない・残さない。
/// 足した記録は `createRecord` で、フォームの保存と同じく端末に入り同期される。
struct FileImportLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let ctx: RenderContext

    @State private var picking = false
    @State private var reading = false
    @State private var pending: Pending?
    @State private var progress: (done: Int, total: Int)?
    @State private var status: String?
    @State private var problem: String?

    /// 確かめてから足すもの
    struct Pending {
        var fileName: String
        var rows: [[String: JSONValue]]
        var message: String
    }

    private var entity: Entity? { rt.dsl.entity(c.entity ?? "") }
    private var types: [UTType] { FileTextReading.contentTypes(c.accept) }
    /// public の entity には足さない(backend の検証でも弾く。古い DSL の保険)
    private var usable: Bool { entity != nil && !types.isEmpty && EntityScope(raw: entity?.scope) != .public && entity?.single != true }

    var body: some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.xs) {
            Button {
                status = nil
                picking = true
            } label: {
                if reading || progress != nil {
                    Label(progressText, systemImage: "hourglass")
                } else {
                    Label(buttonLabel, systemImage: "square.and.arrow.down")
                }
            }
            .buttonStyle(RenderButtonStyle(kind: .secondary))
            .disabled(!usable || reading || progress != nil)
            .accessibilityIdentifier("fileImport.button")
            if let status {
                Text(status)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("fileImport.status")
            }
        }
        .fileImporter(isPresented: $picking, allowedContentTypes: types, allowsMultipleSelection: false) { result in
            guard case .success(let urls) = result, let url = urls.first else { return }
            Task { await read(url) }
        }
        .alert(L("取り込みますか？"), isPresented: Binding(get: { pending != nil }, set: { if !$0 { pending = nil } }), presenting: pending) { p in
            Button(L("追加する")) { Task { await add(p) } }
            Button(L("キャンセル"), role: .cancel) {}
        } message: { p in
            Text(p.message)
        }
        .alert(L("取り込めませんでした"), isPresented: Binding(get: { problem != nil }, set: { if !$0 { problem = nil } })) {
            Button(L("OK"), role: .cancel) {}
        } message: {
            Text(problem ?? "")
        }
    }

    private var buttonLabel: String {
        if let label = c.label, !label.isEmpty { return label }
        return L("ファイルから取り込む")
    }

    private var progressText: String {
        guard let progress else { return L("読み込み中…") }
        return L("追加中 \(progress.done) / \(progress.total)")
    }

    // MARK: - 読む

    private func read(_ url: URL) async {
        guard let entity, let kind = FileTextReading.kind(of: url, accept: c.accept) else {
            problem = L("この種類のファイルは取り込めません")
            return
        }
        reading = true
        defer { reading = false }
        let refs = Set(entity.fields.compactMap(\.refEntity))
        await rt.load(entities: refs.union([entity.id]))
        let scoped = url.startAccessingSecurityScopedResource()
        let data = try? Data(contentsOf: url, options: .mappedIfSafe)
        if scoped { url.stopAccessingSecurityScopedResource() }
        guard let data else {
            problem = L("ファイルを開けませんでした")
            return
        }
        let limit = kind == .pdf ? RecordImport.maxPDFBytes : RecordImport.maxTextBytes
        guard data.count <= limit else {
            problem = L("ファイルが大きすぎます(\(limit / 1_000_000)MB まで)")
            return
        }
        let name = url.lastPathComponent
        switch kind {
        case .csv: planCSV(data, entity: entity, fileName: name)
        case .text, .pdf: await planBody(data, kind: kind, entity: entity, fileName: name)
        }
    }

    private func planCSV(_ data: Data, entity: Entity, fileName: String) {
        guard let text = CSVReader.decode(data) else {
            problem = L("文字コードを読めませんでした(UTF-8 か Shift_JIS の CSV にしてください)")
            return
        }
        let table = CSVReader.parse(text)
        let plan = RecordImport.plan(table: table, entity: entity, map: c.map, existing: rt.records(for: entity.id).count) { field, name in
            refId(field, name)
        }
        guard !plan.matched.isEmpty else {
            let names = entity.storedFields.map(\.name).joined(separator: L("・"))
            problem = L("1 行目の項目名が「\(entity.name)」の項目(\(names))に合いません")
            return
        }
        guard !plan.rows.isEmpty else {
            problem = summary(plan, entity: entity)
            return
        }
        pending = Pending(fileName: fileName, rows: plan.rows, message: summary(plan, entity: entity))
    }

    private func planBody(_ data: Data, kind: FileTextReading.Kind, entity: Entity, fileName: String) async {
        let text = await Task.detached(priority: .userInitiated) {
            kind == .pdf ? FileTextReading.pdfText(data) : RecordImport.plainText(data)
        }.value
        guard let text, !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            problem = kind == .pdf ? L("PDF から文字を取り出せませんでした") : L("文字を読めませんでした")
            return
        }
        guard rt.records(for: entity.id).count < RecordImport.maxRecordsPerEntity,
              let field = c.field, let made = RecordImport.bodyRecord(text: text, fileName: fileName, entity: entity, field: field)
        else {
            problem = L("これ以上記録を足せません")
            return
        }
        let count = (made.data[field]?.stringValue ?? "").count
        var message = L("「\(fileName)」から \(count) 文字を「\(entity.name)」に 1 件追加します。")
        if made.truncated { message += "\n" + L("長いので先頭の \(count) 文字だけ入れます。") }
        pending = Pending(fileName: fileName, rows: [made.data], message: message)
    }

    /// 確認の文: 足す件数・使う列・使わない列・読めない行(最初の 3 つ)・上限で入れない行。
    private func summary(_ plan: ImportPlan, entity: Entity) -> String {
        var lines = [L("「\(entity.name)」に \(plan.rows.count) 件を追加します。")]
        let used: [String] = plan.matched.map { $0.column == $0.field ? $0.column : $0.column + "→" + $0.field }
        lines.append(L("使う列: \(used.joined(separator: L("・")))"))
        let unused = plan.unused.prefix(8).joined(separator: L("・"))
        if !unused.isEmpty { lines.append(L("使わない列: \(unused)")) }
        if !plan.failures.isEmpty {
            lines.append(L("読めない行: \(plan.failures.count) 行(入れません)"))
            for f in plan.failures.prefix(3) { lines.append("・" + failureText(f)) }
        }
        if plan.overLimit > 0 { lines.append(L("上限(1 回 \(RecordImport.maxRows) 件・合計 \(RecordImport.maxRecordsPerEntity) 件)を超える \(plan.overLimit) 行は入れません")) }
        return lines.joined(separator: "\n")
    }

    private func failureText(_ f: ImportRowFailure) -> String {
        switch f.kind {
        case .invalid: L("\(f.line) 行目: \(f.fieldName)「\(f.cell)」を読めません")
        case .required: L("\(f.line) 行目: \(f.fieldName)が空です")
        case .tooLong: L("\(f.line) 行目: \(f.fieldName)が長すぎます")
        }
    }

    /// ref / refs の名前 → 参照先のレコード id(見出しが同じもの)。
    private func refId(_ field: Field, _ name: String) -> String? {
        guard let target = field.refEntity else { return nil }
        return rt.records(for: target).first { rt.refTitle($0.id, entity: target) == name }?.id
    }

    // MARK: - 足す

    private func add(_ p: Pending) async {
        guard let entity else { return }
        progress = (0, p.rows.count)
        var added = 0
        for data in p.rows {
            if await rt.createRecord(entity: entity.id, data: data) { added += 1 }
            progress = (added, p.rows.count)
        }
        progress = nil
        status = added == p.rows.count ? L("\(added) 件を追加しました") : L("\(added) 件を追加しました(\(p.rows.count - added) 件は追加できませんでした)")
    }
}
