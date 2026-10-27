import SwiftUI
import MetamonDSL

/// `randomPicker`(#263): ボタンで entity(+ filter)から 1 件をランダムに選び、titleField(+ subtitleField)を大きく出す。
/// 選んだ id は `@State` にだけ持つ(保存しない・再描画では変わらない)。表示はその id で現在のレコードを引き直すので、
/// 候補を編集すれば名前が追従し、消されたら未選択に戻る。選び方は `RandomPick`(MetamonDSL)。
/// 演出は短いシャッフル(約 0.7 秒)。「視差効果を減らす」では演出せずに結果だけ出す。
struct RandomPickerLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    let c: Component
    let ctx: RenderContext

    @State private var pickedId: String?
    /// シャッフル中に見せている候補(nil = 演出していない)
    @State private var spinning: Record?
    @State private var spinTask: Task<Void, Never>?
    @State private var pickCount = 0

    private var entityId: String { c.entity ?? "" }
    private var entity: Entity? { rt.dsl.entity(entityId) }

    var body: some View {
        let candidates = rt.filteredRecords(c)
        let picked = pickedId.flatMap { id in candidates.first { $0.id == id } }
        VStack(spacing: RenderTheme.Spacing.md) {
            if let title = c.title, !title.isEmpty {
                Text(title)
                    .font(RenderTheme.Fonts.sectionTitle)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .accessibilityAddTraits(.isHeader)
            }
            result(shown: spinning ?? picked, candidates: candidates, isFinal: spinning == nil)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            Button {
                start(candidates)
            } label: {
                Label(buttonLabel, systemImage: "dice.fill")
            }
            .buttonStyle(RenderButtonStyle(kind: .primary))
            .disabled(candidates.isEmpty || spinning != nil)
            .accessibilityIdentifier("randomPicker.button")
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .renderCard(wash: true)
        .sensoryFeedback(.success, trigger: pickCount)
        .onDisappear { spinTask?.cancel() }
    }

    private var buttonLabel: String {
        if let label = c.label, !label.isEmpty { return label }
        return L("ランダムに決める")
    }

    // MARK: - result

    @ViewBuilder
    private func result(shown: Record?, candidates: [Record], isFinal: Bool) -> some View {
        if let r = shown {
            let content = VStack(spacing: RenderTheme.Spacing.xs) {
                Text(title(r))
                    .font(.system(.largeTitle, design: .rounded).weight(.bold))
                    .foregroundStyle(isFinal ? AnyShapeStyle(RenderTheme.brandFill) : AnyShapeStyle(Color.secondary))
                    .multilineTextAlignment(.center)
                    .lineLimit(2)
                    .minimumScaleFactor(0.5)
                if isFinal, let sub = subtitle(r) {
                    Text(sub).font(.subheadline).foregroundStyle(.secondary).lineLimit(1)
                }
            }
            .id(r.id + (isFinal ? "#final" : ""))
            .transition(reduceMotion ? .opacity : .push(from: .bottom))
            if isFinal, c.onItemTap != nil {
                Button { open(r) } label: { content.contentShape(Rectangle()) }
                    .buttonStyle(.plain)
                    .accessibilityElement(children: .combine)
                    .accessibilityIdentifier("randomPicker.result")
                    .accessibilityValue(title(r))
                    .accessibilityHint(L("タップで詳しく見る"))
            } else {
                content
                    .accessibilityElement(children: .combine)
                    .accessibilityIdentifier("randomPicker.result")
                    .accessibilityValue(isFinal ? title(r) : "")
            }
        } else if candidates.isEmpty {
            EmptyPlaceholder(icon: "dice", emptyText: L("候補がありません"), state: rt.emptyState(for: entityId)) {
                Task { await rt.load(entities: [entityId], force: true) }
            }
        } else {
            VStack(spacing: RenderTheme.Spacing.sm) {
                Image(systemName: "questionmark")
                    .font(.system(size: 34, weight: .bold, design: .rounded))
                    .foregroundStyle(RenderTheme.Colors.brandA.opacity(0.6))
                    .frame(width: 60, height: 60)
                    .background(RenderTheme.Colors.brandSoft, in: Circle())
                    .accessibilityHidden(true)
                Text(L("ボタンを押すと 1 つ選びます"))
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
            }
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("randomPicker.result")
        }
    }

    // MARK: - pick

    private func start(_ candidates: [Record]) {
        guard let winner = RandomPick.pick(from: candidates, excluding: pickedId) else { return }
        spinTask?.cancel()
        var g = SystemRandomNumberGenerator()
        let frames = reduceMotion ? [] : RandomPick.reel(from: candidates, ending: winner, frames: 9, using: &g)
        guard !frames.isEmpty else {
            finish(winner)
            return
        }
        spinTask = Task { @MainActor in
            for (i, f) in frames.enumerated() {
                withAnimation(.easeOut(duration: 0.08)) { spinning = f }
                // だんだん遅くなる(0.04 秒 → 0.12 秒)
                try? await Task.sleep(for: .milliseconds(40 + i * 10))
                if Task.isCancelled { return }
            }
            finish(winner)
        }
    }

    private func finish(_ winner: Record) {
        withAnimation(reduceMotion ? nil : .spring(response: 0.35, dampingFraction: 0.7)) {
            spinning = nil
            pickedId = winner.id
        }
        pickCount += 1
        AccessibilityNotification.Announcement(title(winner)).post()
    }

    // MARK: - values / actions

    private func title(_ r: Record) -> String {
        let f = c.titleField ?? entity?.displayTitleField ?? ""
        let s = rt.display(r[f], field: entity?.field(f))
        return s.isEmpty ? L("(無題)") : s
    }

    private func subtitle(_ r: Record) -> String? {
        guard let f = c.subtitleField, !f.isEmpty else { return nil }
        let s = rt.display(r[f], field: entity?.field(f))
        return s.isEmpty || s == FieldCoding.unavailable ? nil : s
    }

    private func open(_ r: Record) {
        guard let action = c.onItemTap else { return }
        Task { await rt.run(action, ctx.actionContext(recordId: r.id)) }
    }
}
