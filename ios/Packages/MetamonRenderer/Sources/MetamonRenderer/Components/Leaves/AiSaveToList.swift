import SwiftUI
import MetamonDSL

/// `aiButton.saveTo`: generate の list の結果を 1 行ずつ「保存」/ 下の「全部保存」で entity に足す。
/// 押すまで何も保存しない。保存は `createRecord`(フォームの保存と同じ経路: 端末の DB → 同期の列)なので、
/// 共有データの決まり・プランの上限・表示できない言葉はサーバーと同期の列がいつもどおり扱う(拒否された行は列が元に戻して知らせる)。
/// 足した記録の取り消しは一覧の削除(「元に戻す」つき)で行う。
struct AiSaveToList: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let saveTo: AISaveTo
    let items: [String]
    @State private var saved: Set<Int> = []
    @State private var failed: Set<Int> = []
    @State private var saving = false
    @State private var status: String?

    private var entity: Entity? { rt.dsl.entity(saveTo.entity) }
    private var unsaved: [Int] { items.indices.filter { !saved.contains($0) } }

    var body: some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
                ForEach(Array(items.enumerated()), id: \.offset) { i, item in
                    row(i, item)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(RenderTheme.Spacing.md)
            .background(RoundedRectangle(cornerRadius: 12).fill(Color.secondary.opacity(0.08)))
            .textSelection(.enabled)
            .accessibilityIdentifier("aiGenerate.result")
            if unsaved.count > 1 {
                Button {
                    Task { await save(unsaved) }
                } label: {
                    Label(L("全部保存"), systemImage: "tray.and.arrow.down").frame(maxWidth: .infinity)
                }
                .buttonStyle(RenderButtonStyle(kind: .secondary))
                .disabled(saving || entity == nil)
                .accessibilityHint(L("AI の結果を「\(entity?.name ?? saveTo.entity)」に保存"))
                .accessibilityIdentifier("aiGenerate.saveAll")
            }
            if let status {
                Text(status).font(.caption).foregroundStyle(failed.isEmpty ? Color.secondary : Color.red)
                    .accessibilityIdentifier("aiGenerate.saveStatus")
            }
        }
    }

    private func row(_ i: Int, _ item: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: RenderTheme.Spacing.sm) {
            Text("・").foregroundStyle(.secondary)
            Text(item).frame(maxWidth: .infinity, alignment: .leading)
                .foregroundStyle(saved.contains(i) ? .secondary : .primary)
            if saved.contains(i) {
                Label(L("保存済み"), systemImage: "checkmark")
                    .labelStyle(.iconOnly).foregroundStyle(.secondary)
                    .accessibilityLabel(L("保存済み"))
            } else {
                Button(L("保存")) { Task { await save([i]) } }
                    .font(.footnote.weight(.semibold))
                    .buttonStyle(.borderless)
                    .disabled(saving || entity == nil)
                    .accessibilityIdentifier("aiGenerate.save.\(i)")
                if failed.contains(i) {
                    Image(systemName: "exclamationmark.circle").foregroundStyle(.red)
                        .accessibilityLabel(L("保存できませんでした"))
                }
            }
        }
        .accessibilityElement(children: .combine)
    }

    private func save(_ indices: [Int]) async {
        guard let entity, !saving else { return }
        saving = true
        defer { saving = false }
        await rt.load(entities: [entity.id])
        let room = AISaveTo.room(existing: rt.records(for: entity.id).count, wanted: indices.count)
        var added = 0
        var notAdded = 0
        for (n, i) in indices.enumerated() {
            guard n < room, let data = saveTo.record(items[i], in: entity) else {
                failed.insert(i)
                notAdded += 1
                continue
            }
            if await rt.createRecord(entity: entity.id, data: data) {
                withAnimation(RenderTheme.Motion.select) { _ = saved.insert(i) }
                failed.remove(i)
                added += 1
            } else {
                failed.insert(i)
                notAdded += 1
            }
        }
        if room < indices.count, added == 0 {
            status = L("これ以上記録を足せません")
        } else {
            status = notAdded == 0 ? L("\(added) 件を追加しました") : L("\(added) 件を追加しました(\(notAdded) 件は追加できませんでした)")
        }
        if let status { AccessibilityNotification.Announcement(status).post() }
    }
}
