import SwiftUI
import UIKit
import UniformTypeIdentifiers
import MetamonDSL

/// `fileImport`: ボタンでファイルを選び、entity に記録を足す。
/// - csv: 1 行 = 1 件(列の当て方・型の変換は `RecordImport`)。足す前に「n 件を追加・m 行は読めない」を確かめる。
/// - text / pdf: 本文を取り出して `field` に入れた記録を 1 件(PDF の画像だけのページは端末内の文字認識)。
/// - audio: 録音を端末内で文字にして(`AudioFileTranscriptionHost`・30 分 / 200MB まで)、`field` に入れた記録を 1 件。
///   前面にある間だけ進め(画面は消さない)、背面に回った・画面を離れた・やめるで止める。
/// ファイルはシステムのファイル選択(別プロセス・権限の確認なし)から読むだけで、送らない・コピーしない・残さない。
/// 足した記録は `createRecord` で、フォームの保存と同じく端末に入り同期される。
struct FileImportLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.scenePhase) private var scenePhase
    let c: Component
    let ctx: RenderContext

    @State private var picking = false
    @State private var reading = false
    /// 録音の文字起こしの進み具合(nil = していない)
    @State private var transcribing: Double?
    @State private var job: Task<Void, Never>?
    @State private var leftForeground = false
    /// 背面に回った時、今の区切りで止めるまでの猶予
    @State private var grace = BackgroundGrace()
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
            if let transcribing {
                FileImportAudioProgress(progress: transcribing) { job?.cancel() }
            } else {
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
                if c.accept.contains("audio"), !reading {
                    FileImportAudioModelRow(host: rt.audioFileTranscription)
                }
            }
            if let status {
                Text(status)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("fileImport.status")
            }
        }
        .fileImporter(isPresented: $picking, allowedContentTypes: types, allowsMultipleSelection: false) { result in
            guard case .success(let urls) = result, let url = urls.first else { return }
            job = Task {
                await read(url)
                job = nil
            }
        }
        .onDisappear { job?.cancel() }
        .onChange(of: scenePhase) { _, phase in
            // 文字起こしは前面だけ(背面ではモデルが止められることがある)。戻ってから選び直してもらう。
            // 止まるまで(長くても 1 区切り)を背面の猶予で待つ。
            if phase == .background, transcribing != nil {
                leftForeground = true
                grace.begin("fileImport.transcribe")
                job?.cancel()
            }
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
        if kind == .audio {
            await transcribe(url, entity: entity)
            return
        }
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
        case .audio: break
        }
    }

    /// 録音を端末内で文字にする。ファイルは読むだけ(コピーしない)で、文字にした後は確かめてから 1 件足す。
    private func transcribe(_ url: URL, entity: Entity) async {
        guard let host = rt.audioFileTranscription else {
            problem = L("この画面では録音ファイルを文字にできません")
            return
        }
        guard host.audioFileReadiness == .ready else {
            problem = L("音声モデルの準備ができていません。準備ができてから、もう一度選んでください")
            return
        }
        let scoped = url.startAccessingSecurityScopedResource()
        defer { if scoped { url.stopAccessingSecurityScopedResource() } }
        let size = await AudioFileImport.measure(url)
        if let issue = AudioFileImport.check(bytes: size.bytes, seconds: size.seconds) {
            problem = switch issue {
            case .tooLarge: L("ファイルが大きすぎます(\(AudioFileImport.maxBytes / 1_000_000)MB まで)")
            case .tooLong: L("録音が長すぎます(\(Int(AudioFileImport.maxSeconds) / 60) 分まで)。分けた録音を選んでください")
            case .unreadable, .empty: L("この録音ファイルは読めませんでした")
            }
            return
        }
        leftForeground = false
        transcribing = 0
        let awake = ScreenAwake.shared.hold() // 文字にしている間は画面を消さない(前面だけで進むため)
        defer {
            transcribing = nil
            awake.release()
            grace.end()
        }
        do {
            let text = try await host.transcribeAudioFile(url) { transcribing = $0 }
            guard !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
                problem = L("録音から声を聞き取れませんでした")
                return
            }
            planText(text, entity: entity, fileName: url.lastPathComponent, note: AudioFileImport.durationText(size.seconds ?? 0))
        } catch is CancellationError {
            status = leftForeground ? L("アプリを離れたので文字起こしをやめました。もう一度選んでください") : L("文字起こしをやめました")
        } catch {
            problem = error.localizedDescription
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
        planText(text, entity: entity, fileName: fileName)
    }

    /// 本文(または文字起こし)を `field` に入れた 1 件を、確かめてから足す。`note` は録音の長さなど。
    private func planText(_ text: String, entity: Entity, fileName: String, note: String? = nil) {
        guard rt.records(for: entity.id).count < RecordImport.maxRecordsPerEntity,
              let field = c.field, let made = RecordImport.bodyRecord(text: text, fileName: fileName, entity: entity, field: field)
        else {
            problem = L("これ以上記録を足せません")
            return
        }
        let count = (made.data[field]?.stringValue ?? "").count
        var message = L("「\(fileName)」から \(count) 文字を「\(entity.name)」に 1 件追加します。")
        if made.truncated { message += "\n" + L("長いので先頭の \(count) 文字だけ入れます。") }
        if let note { message += "\n" + L("録音の長さ: \(note)") }
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
