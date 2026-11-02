import SwiftUI
import MetamonDSL

/// `flashcards`(#264): entity のレコード(+ filter)を 1 枚ずつ出す単語カード。タップで表(front)と裏(back)をめくる。
/// 右スワイプ = 覚えた・左 = まだ を toggle(boolean)へ保存して次へ(`list.item.toggle` と同じ `updateRecord`・楽観的 UI)。
/// スワイプの代わりに「まだ」「覚えた」ボタンも出す(VoiceOver・スイッチコントロール向け)。
/// 進み具合は `FlashcardDeck`(MetamonDSL)を `@State` に持つだけで保存しない。
struct FlashcardsLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    let c: Component
    let ctx: RenderContext

    @State private var deck = FlashcardDeck()
    @State private var drag: CGFloat = 0
    @State private var answers = 0

    private var entityId: String { c.entity ?? "" }
    private var entity: Entity? { rt.dsl.entity(entityId) }
    /// スワイプで答えになる距離
    private let threshold: CGFloat = 90

    var body: some View {
        let cards = Self.cards(c, rt: rt, pending: deck.pending)
        let card = deck.current(in: cards)
        VStack(spacing: RenderTheme.Spacing.md) {
            if let card {
                let p = deck.progress(in: cards)
                Text(L("\(p.position) / \(p.total) 枚"))
                    .font(.subheadline.monospacedDigit())
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity)
                    .accessibilityLabel(L("\(p.total) 枚中 \(p.position) 枚目"))
                    .accessibilityIdentifier("flashcards.progress")
                face(card)
                buttons(card)
            } else if deck.isDone(in: cards) {
                done
            } else {
                EmptyPlaceholder(icon: "rectangle.on.rectangle.angled", emptyText: emptyText, state: rt.emptyState(for: entityId)) {
                    Task { await rt.load(entities: [entityId], force: true) }
                }
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .renderCard(wash: true)
        .sensoryFeedback(.selection, trigger: answers)
    }

    private var emptyText: String {
        Self.learnedFilter(c) == false ? L("覚えていないカードはありません") : L("カードがありません")
    }

    // MARK: - カード

    private func face(_ card: Record) -> some View {
        let text = deck.flipped ? value(card, c.back) : value(card, c.front)
        return Button {
            withAnimation(reduceMotion ? nil : .easeInOut(duration: 0.3)) { deck.flip() }
        } label: {
            VStack(spacing: RenderTheme.Spacing.xs) {
                Text(deck.flipped ? fieldName(c.back) : fieldName(c.front))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Text(text)
                    .font(.system(.title, design: .rounded).weight(.bold))
                    .foregroundStyle(deck.flipped ? AnyShapeStyle(RenderTheme.brandFill) : AnyShapeStyle(Color.primary))
                    .multilineTextAlignment(.center)
                    .minimumScaleFactor(0.5)
            }
            // 裏返した面の文字が鏡文字にならないよう、中身だけ戻す
            .rotation3DEffect(.degrees(deck.flipped && !reduceMotion ? 180 : 0), axis: (0, 1, 0))
            .padding(RenderTheme.Spacing.md)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(RenderTheme.Colors.brandSoft.opacity(deck.flipped ? 1 : 0.5), in: RoundedRectangle(cornerRadius: 16))
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .rotation3DEffect(.degrees(deck.flipped && !reduceMotion ? 180 : 0), axis: (0, 1, 0))
        .overlay(alignment: .top) { swipeHint }
        .offset(x: drag)
        .rotationEffect(.degrees(reduceMotion ? 0 : Double(drag / 25)))
        .simultaneousGesture(swipe(card))
        .id(card.id)
        .transition(reduceMotion ? .opacity : .asymmetric(insertion: .scale(scale: 0.9).combined(with: .opacity), removal: .opacity))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(text)
        .accessibilityValue(deck.flipped ? L("裏") : L("表"))
        .accessibilityHint(L("ダブルタップでめくる"))
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("flashcards.card")
    }

    /// ドラッグ中だけ「覚えた」「まだ」を出す。
    @ViewBuilder
    private var swipeHint: some View {
        if abs(drag) > 20 {
            Text(drag > 0 ? L("覚えた") : L("まだ"))
                .font(.headline)
                .padding(.horizontal, RenderTheme.Spacing.sm)
                .padding(.vertical, RenderTheme.Spacing.xs)
                .background(.thinMaterial, in: Capsule())
                .padding(RenderTheme.Spacing.sm)
                .opacity(min(1, Double(abs(drag) / threshold)))
                .accessibilityHidden(true)
        }
    }

    private func swipe(_ card: Record) -> some Gesture {
        DragGesture(minimumDistance: 20)
            .onChanged { v in
                guard abs(v.translation.width) > abs(v.translation.height) else { return }
                drag = v.translation.width
            }
            .onEnded { v in
                let w = v.translation.width
                if abs(w) >= threshold, abs(w) > abs(v.translation.height) {
                    answer(card, learned: w > 0)
                } else {
                    withAnimation(.spring(response: 0.3, dampingFraction: 0.8)) { drag = 0 }
                }
            }
    }

    private func buttons(_ card: Record) -> some View {
        HStack(spacing: RenderTheme.Spacing.sm) {
            Button { answer(card, learned: false) } label: {
                Label(L("まだ"), systemImage: "arrow.uturn.left")
            }
            .buttonStyle(RenderButtonStyle(kind: .secondary))
            .accessibilityIdentifier("flashcards.notYet")
            Button { answer(card, learned: true) } label: {
                Label(L("覚えた"), systemImage: "checkmark")
            }
            .buttonStyle(RenderButtonStyle(kind: .primary))
            .accessibilityIdentifier("flashcards.learned")
        }
    }

    private var done: some View {
        VStack(spacing: RenderTheme.Spacing.sm) {
            Image(systemName: "checkmark.seal.fill")
                .font(.system(size: 40))
                .foregroundStyle(RenderTheme.brandFill)
                .accessibilityHidden(true)
            Text(L("全部終わりました"))
                .font(.headline)
            Button {
                withAnimation(reduceMotion ? nil : .easeInOut) { deck.restart() }
            } label: {
                Label(L("もう一度"), systemImage: "arrow.counterclockwise")
            }
            .buttonStyle(RenderButtonStyle(kind: .secondary))
            .accessibilityIdentifier("flashcards.restart")
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityIdentifier("flashcards.done")
    }

    // MARK: - 答える

    /// 次のカードへ進め、toggle があれば保存する(失敗したら保存待ちを外す = ストアの値に戻る)。
    private func answer(_ card: Record, learned: Bool) {
        if c.toggle != nil, !rt.canEdit(entity: entityId, record: card) {
            rt.notifyReadOnly(entity: entityId)
            return
        }
        var next = deck
        let patch = next.answer(card, learned: learned, toggle: Self.toggleField(c, in: entity))
        withAnimation(reduceMotion ? nil : .easeOut(duration: 0.25)) {
            deck = next
            drag = 0
        }
        answers += 1
        AccessibilityNotification.Announcement(learned ? L("覚えた") : L("まだ")).post()
        guard let patch else { return }
        Task {
            let saved = await rt.updateRecord(entity: entityId, recordId: card.id, data: patch)
            if !saved { print("[Metamon] flashcards not saved: \(entityId)/\(card.id)") }
            deck.settle(card.id, requested: learned)
        }
    }

    // MARK: - 値

    private func value(_ r: Record, _ f: String?) -> String {
        guard let f, !f.isEmpty else { return "" }
        let s = rt.display(r[f], field: entity?.field(f))
        return s.isEmpty ? "—" : s
    }

    private func fieldName(_ f: String?) -> String {
        f.flatMap { entity?.field($0)?.name } ?? ""
    }

    // MARK: - 山札(描画の木 RenderTree と共用)

    /// toggle に使える field(boolean・computed でない)。条件を満たさなければ nil(めくるだけ)。
    static func toggleField(_ c: Component, in entity: Entity?) -> String? {
        guard let id = c.toggle, let f = entity?.field(id), f.computed == nil, f.type == .boolean else { return nil }
        return id
    }

    /// toggle の field を boolean で比べる filter が出す側(覚えた = true / まだ = false)。無ければ nil。
    static func learnedFilter(_ c: Component) -> Bool? {
        c.filters.lazy.compactMap { FlashcardDeck.learnedFilter($0, toggle: c.toggle) }.first
    }

    /// 出題するカード。toggle を比べる filter は「値の無いカード = まだ」で評価するため、ほかの filter だけをランタイムに渡す。
    static func cards(_ c: Component, rt: any DSLRuntime, pending: [String: Bool]) -> [Record] {
        let learned = learnedFilter(c)
        var base = c
        if learned != nil {
            base.filters = c.filters.filter { FlashcardDeck.learnedFilter($0, toggle: c.toggle) == nil }
            base.filter = base.filters.first
        }
        return FlashcardDeck.cards(rt.filteredRecords(base), toggle: c.toggle, learned: learned, pending: pending)
    }
}
