import SwiftUI
import MetamonRenderer

/// マイク FAB(= Tailor It 本体)。ピクチャーインピクチャー風に動かせる(#52)。
/// - タップ = 「このアプリを変える」発話シート、長押し(0.45s) = メタ操作(変身一覧)。
/// - ドラッグで自由移動 → 離すと最寄りの左右の辺に吸着。端から `fabSize * 0.3` 以内で離す(or 端へ弾く)と画面外へ隠れ、取っ手だけ残る。
///   取っ手のタップ / 内側へ 20pt 引く で戻る。取っ手の長押しでも変身一覧。
///   初めて隠した時だけ、取っ手の横に「取っ手を押すと戻ります」を数秒出す(UX 点検 L6・`FabHideHint`)。
/// - 長押し・ドラッグを知らない人には、FAB の上に吹き出しを 1 回ずつ出す(#1441・`CoachMarkBubble`)。
/// - 位置(辺・下端距離・隠れ)は `@AppStorage` でアプリ共通に保存。初期値は左下 + 自動退避(従来どおり)。
/// - 吸着した側の最下段 2 列に DSL の部品がある画面では、その上端まで退避する(#83・左 `GridBottomObstructionKey` / 右 `GridBottomTrailingObstructionKey`)。
///
/// ジェスチャ: `Button`(VoiceOver・押下ハイライト)に長押しとドラッグを simultaneous で重ねる。
/// ドラッグは global 座標で取る(FAB が指と一緒に動くのでローカルだと値がぶれる)。FAB が指に付いてくるので離した時に Button のタップも発火するため、
/// ドラッグ認識中 / 長押し発火後は `suppressTap` / `longPressFired` でタップを捨てる。長押し発火後のドラッグは無視する。
struct VoiceFab: View {
    let hasTabs: Bool
    /// 最下段 A〜B 列 / E〜F 列に掛かる部品の上端(global y)。無ければ nil
    let leadingObstructionTop: CGFloat?
    let trailingObstructionTop: CGFloat?
    /// 生成中は画面と一緒に沈めて触れなくする
    let isBusy: Bool
    let onTap: () -> Void
    let onLongPress: () -> Void
    /// 気づきにくい操作の吹き出し(#1441)。出すものが無ければ nil
    var coachMark: CoachMark? = nil
    var onCoachMark: (CoachMark, CoachMarkEvent) -> Void = { _, _ in }
    /// 実際に長押し・ドラッグした(吹き出しを次から出さない)
    var onGesturePerformed: (CoachMark) -> Void = { _ in }

    @AppStorage("fab.side") private var sideRaw = Theme.Layout.fabDefaultSide.rawValue
    /// 領域の下端から FAB の下端まで。`FabDock.autoBottom`(-1)= 自動(タブバー・部品退避のみ)
    @AppStorage("fab.bottom") private var storedBottom = Double(FabDock.autoBottom)
    @AppStorage("fab.hidden") private var hidden = false
    /// 隠した時の案内を出したか(L6・アプリ共通で 1 回)
    @AppStorage("fab.hideHintShown") private var hideHintShown = false
    @State private var showsHideHint = false
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    @State private var drag: CGSize = .zero
    @State private var isDragging = false
    @State private var suppressTap = false
    @State private var longPressFired = false
    @State private var handlePull: CGFloat = 0

    private let size = Theme.Layout.fabSize
    private var side: FabSide { FabSide(rawValue: sideRaw) ?? Theme.Layout.fabDefaultSide }
    private var snapAnimation: Animation { reduceMotion ? .easeOut(duration: 0.12) : .spring(duration: 0.38, bounce: 0.22) }

    var body: some View {
        GeometryReader { geo in
            let area = geo.size
            let maxBottom = area.height - size - Theme.Layout.fabTopReserve
            let autoInset = Theme.Layout.fabBottomInset(
                hasTabs: hasTabs,
                obstructionTop: side == .leading ? leadingObstructionTop : trailingObstructionTop,
                rootBottom: geo.frame(in: .global).maxY
            )
            let bottom = FabDock.displayedBottom(stored: CGFloat(storedBottom), autoInset: autoInset, maxBottom: maxBottom)
            let rest = CGPoint(
                x: side == .leading ? Theme.Layout.fabHorizontalInset + size / 2 : area.width - Theme.Layout.fabHorizontalInset - size / 2,
                y: area.height - bottom - size / 2
            )
            ZStack {
                fab(area: area, rest: rest, maxBottom: maxBottom)
                    .position(
                        x: hidden ? (side == .leading ? -size : area.width + size) : rest.x + drag.width,
                        y: rest.y + drag.height
                    )
                if let coachMark, !hidden, !isDragging {
                    coachBubble(coachMark, rest: rest, area: area)
                        .transition(.opacity.combined(with: .scale(scale: 0.92, anchor: side == .leading ? .bottomLeading : .bottomTrailing)))
                }
                if hidden {
                    handle
                        .position(
                            x: side == .leading ? Theme.Layout.fabHandleHitWidth / 2 + handlePull : area.width - Theme.Layout.fabHandleHitWidth / 2 - handlePull,
                            y: rest.y
                        )
                        .transition(.opacity)
                    if showsHideHint {
                        hideHint
                            .frame(maxWidth: .infinity, alignment: side == .leading ? .leading : .trailing)
                            .padding(.horizontal, Theme.Layout.fabHandleHitWidth + 4)
                            .position(x: area.width / 2, y: rest.y)
                            .transition(.opacity)
                    }
                }
            }
            .animation(.easeOut(duration: 0.25), value: bottom)
            .animation(.easeInOut(duration: 0.25), value: showsHideHint)
            .animation(.easeInOut(duration: 0.25), value: coachMark)
            // 止まっている位置の矩形(global)。Renderer が背の高い list の末尾に FAB ぶんの余白を足すのに使う(#157)。
            // ドラッグ中の位置は流さない(指に合わせて list の余白が揺れるため)。隠れている時は nil = 余白不要
            .preference(key: VoiceFabFrameKey.self, value: hidden ? nil : CGRect(
                x: geo.frame(in: .global).minX + rest.x - size / 2,
                y: geo.frame(in: .global).minY + rest.y - size / 2,
                width: size, height: size
            ))
        }
        // 生成中は画面と一緒に沈めて触れなくする(本体だけ薄くなり FAB だけ浮いて見えるのを防ぐ)
        .opacity(isBusy ? 0.35 : 1)
        .allowsHitTesting(!isBusy)
        .animation(.easeInOut(duration: 0.3), value: isBusy)
        #if DEBUG
        .task { applyDebugPosition() }
        #endif
    }

    // MARK: 本体

    private func fab(area: CGSize, rest: CGPoint, maxBottom: CGFloat) -> some View {
        Button {
            if suppressTap || longPressFired {
                suppressTap = false
                longPressFired = false
                return
            }
            onTap()
        } label: {
            Image(systemName: "mic.fill")
                .font(.title2.weight(.semibold))
                .foregroundStyle(.white)
                .frame(width: size, height: size)
                .background(Theme.brandGradient, in: Circle())
                .overlay(Circle().strokeBorder(.white.opacity(0.25)))
                .shadow(color: Theme.Colors.brandA.opacity(0.45), radius: 14, y: 6)
                .scaleEffect(isDragging ? 1.08 : 1)
                .animation(snapAnimation, value: isDragging)
        }
        .simultaneousGesture(
            // 許容移動はドラッグのしきい値と同じ 10pt(先に動けば長押しは失敗 = ドラッグ扱い)
            LongPressGesture(minimumDuration: Theme.Layout.fabLongPressSeconds, maximumDistance: FabDock.dragThreshold)
                .onChanged { _ in
                    // 指が触れた瞬間: 前回の抑止を持ち越さない
                    guard !isDragging else { return }
                    longPressFired = false
                    suppressTap = false
                }
                .onEnded { _ in
                    guard !isDragging else { return }
                    longPressFired = true
                    onGesturePerformed(.fabLongPress)
                    onLongPress()
                }
        )
        .simultaneousGesture(
            DragGesture(minimumDistance: FabDock.dragThreshold, coordinateSpace: .global)
                .onChanged { v in
                    guard !longPressFired else { return }
                    if !isDragging {
                        isDragging = true
                        suppressTap = true
                    }
                    drag = v.translation
                }
                .onEnded { v in
                    guard isDragging else { return }
                    let snap = FabDock.snap(
                        centerX: rest.x + v.translation.width,
                        predictedCenterX: rest.x + v.predictedEndTranslation.width,
                        width: area.width, fabSize: size
                    )
                    let newBottom = FabDock.storedBottom(
                        centerY: rest.y + v.translation.height, height: area.height, fabSize: size,
                        minBottom: Theme.Layout.fabBottomInset(hasTabs: hasTabs), maxBottom: maxBottom
                    )
                    let wasHidden = hidden
                    onGesturePerformed(.fabDrag)
                    withAnimation(snapAnimation) {
                        isDragging = false
                        sideRaw = snap.side.rawValue
                        storedBottom = Double(newBottom)
                        hidden = snap.hidden
                        drag = .zero
                    }
                    noteHidden(was: wasHidden)
                    // 指を離した瞬間に来る Button のタップを捨ててから解除
                    DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) { suppressTap = false }
                }
        )
        .accessibilityLabel("話しかけて変える")
        .accessibilityHint(L10n.key("長押しで変身一覧", review: "長押しで記録帳の一覧"))
        .accessibilityIdentifier("voice.fab")
        .accessibilityAction(named: L10n.key("変身一覧", review: "記録帳の一覧")) {
            onGesturePerformed(.fabLongPress)
            onLongPress()
        }
        .accessibilityAction(named: "反対側へ移動") {
            onGesturePerformed(.fabDrag)
            withAnimation(snapAnimation) { sideRaw = (side == .leading ? FabSide.trailing : .leading).rawValue }
        }
        .accessibilityAction(named: "端に隠す") {
            onGesturePerformed(.fabDrag)
            let wasHidden = hidden
            withAnimation(snapAnimation) { hidden = true }
            noteHidden(was: wasHidden)
        }
        .allowsHitTesting(!hidden)
        .accessibilityHidden(hidden)
    }

    // MARK: 取っ手(隠れている時)

    private var handle: some View {
        let inner: CGFloat = Theme.Layout.fabHandleWidth / 2
        let shape = UnevenRoundedRectangle(
            topLeadingRadius: side == .leading ? 0 : inner,
            bottomLeadingRadius: side == .leading ? 0 : inner,
            bottomTrailingRadius: side == .leading ? inner : 0,
            topTrailingRadius: side == .leading ? inner : 0,
            style: .continuous
        )
        return Image(systemName: side == .leading ? "chevron.compact.right" : "chevron.compact.left")
            .font(.caption.weight(.bold))
            .foregroundStyle(.white)
            .frame(width: Theme.Layout.fabHandleWidth, height: Theme.Layout.fabHandleHeight)
            .background(Theme.brandGradient, in: shape)
            .shadow(color: Theme.Colors.brandA.opacity(0.4), radius: 8, y: 3)
            // 触れる範囲は見た目より広く(端に貼り付いた細い部品は押しにくい)
            .frame(width: Theme.Layout.fabHandleHitWidth, height: Theme.Layout.fabHandleHeight + 16,
                   alignment: side == .leading ? .leading : .trailing)
            .contentShape(Rectangle())
            .onTapGesture { reveal() }
            .onLongPressGesture(minimumDuration: Theme.Layout.fabLongPressSeconds, maximumDistance: FabDock.dragThreshold) {
                onGesturePerformed(.fabLongPress)
                onLongPress()
            }
            .simultaneousGesture(
                DragGesture(minimumDistance: FabDock.dragThreshold, coordinateSpace: .global)
                    .onChanged { v in
                        let inward = side == .leading ? v.translation.width : -v.translation.width
                        handlePull = min(max(0, inward), FabDock.revealDistance * 1.5)
                    }
                    .onEnded { v in
                        if FabDock.shouldReveal(translationX: v.translation.width, side: side) {
                            reveal()
                        } else {
                            withAnimation(snapAnimation) { handlePull = 0 }
                        }
                    }
            )
            .accessibilityElement()
            .accessibilityLabel("話しかけて変える。ダブルタップで表示")
            .accessibilityIdentifier("voice.fabHandle")
            .accessibilityAddTraits(.isButton)
            .accessibilityAction { reveal() }
            .accessibilityAction(named: L10n.key("変身一覧", review: "記録帳の一覧")) {
                onGesturePerformed(.fabLongPress)
                onLongPress()
            }
    }

    private func reveal() {
        showsHideHint = false
        withAnimation(snapAnimation) {
            hidden = false
            handlePull = 0
        }
    }

    // MARK: 吹き出し(#1441)

    /// FAB の真上に出し、矢印の先を FAB の中心に合わせる。
    private func coachBubble(_ mark: CoachMark, rest: CGPoint, area: CGSize) -> some View {
        let gap: CGFloat = 8
        let bottom = max(0, rest.y - size / 2 - gap)
        let edgeInset = Theme.Spacing.md
        return CoachMarkBubble(
            mark: mark,
            arrowAlignment: side == .leading ? .leading : .trailing,
            arrowInset: Theme.Layout.fabHorizontalInset + size / 2 - edgeInset,
            onEvent: { onCoachMark(mark, $0) }
        )
        .padding(.horizontal, edgeInset)
        .frame(width: area.width, height: bottom, alignment: side == .leading ? .bottomLeading : .bottomTrailing)
        .position(x: area.width / 2, y: bottom / 2)
    }

    // MARK: 隠した時の案内(L6)

    private var hideHint: some View {
        Text(FabHideHint.message)
            .font(.footnote.weight(.semibold))
            .foregroundStyle(.white)
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .background(Color.black.opacity(0.78), in: Capsule())
            .fixedSize()
            .onTapGesture { showsHideHint = false }
            .accessibilityHidden(true) // VoiceOver には読み上げで伝える
    }

    /// 初めて隠した時だけ案内を数秒出す。
    private func noteHidden(was wasHidden: Bool) {
        guard FabHideHint.shouldShow(wasHidden: wasHidden, nowHidden: hidden, alreadyShown: hideHintShown) else { return }
        hideHintShown = true
        showsHideHint = true
        AccessibilityNotification.Announcement(FabHideHint.message).post()
        Task { @MainActor in
            try? await Task.sleep(for: .seconds(FabHideHint.seconds))
            showsHideHint = false
        }
    }

    #if DEBUG
    /// ヘッドレス E2E / スクショ用: `METAMON_FAB_POSITION=left|right[-hidden][:<下端距離>]`(例 `right-hidden`・`right:320`)。
    /// ドラッグは simctl で再現しにくいので保存値を直接書く。`auto` で初期状態(左下・自動)に戻す。
    private func applyDebugPosition() {
        guard let spec = ProcessInfo.processInfo.environment["METAMON_FAB_POSITION"], !spec.isEmpty else { return }
        if spec == "auto" {
            sideRaw = Theme.Layout.fabDefaultSide.rawValue
            storedBottom = Double(FabDock.autoBottom)
            hidden = false
            return
        }
        let parts = spec.split(separator: ":")
        let pos = parts.first.map(String.init) ?? ""
        sideRaw = (pos.hasPrefix("right") ? FabSide.trailing : .leading).rawValue
        hidden = pos.hasSuffix("-hidden")
        storedBottom = parts.count > 1 ? Double(parts[1]) ?? Double(FabDock.autoBottom) : Double(FabDock.autoBottom)
    }
    #endif
}

/// FAB を端に隠した時の案内(L6)。一覧・設定・直すの入口がすべて FAB なので、偶然隠しても戻し方が分かるように 1 回だけ出す。
enum FabHideHint {
    static let seconds: Double = 4
    static var message: String { L10n.tr("取っ手を押すと戻ります") }

    /// 出ていた FAB を隠した時で、まだ案内していない時だけ
    static func shouldShow(wasHidden: Bool, nowHidden: Bool, alreadyShown: Bool) -> Bool {
        !wasHidden && nowHidden && !alreadyShown
    }
}

/// `VoiceFab` の止まっている位置(global)。隠れている / 出していない時は nil。RootView が Renderer の `fabFrame` に渡す(#157)。
struct VoiceFabFrameKey: PreferenceKey {
    static let defaultValue: CGRect? = nil
    static func reduce(value: inout CGRect?, nextValue: () -> CGRect?) {
        value = nextValue() ?? value
    }
}
