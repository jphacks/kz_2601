import SwiftUI
import MetamonServices

/// 生成・改変の前の対話(#75 / チャット基本化 #186)。初期画面(create)と、作ったアプリの音声シート(edit)で最初の発話のあとに出る。
/// - 吹き出し: 右 = ユーザー(音声の書き起こし / 文字)、左 = AI。最新の質問だけ大きく
/// - kind ごと(`ClarifyFlow.offer`): answer = 吹き出しだけ / question = 選択肢チップ /
///   proposal = 案のカード(各「これでいく」)/ ready = 要約カード +「これでいく」
/// - 副ボタン(`ClarifyFlow.shortcut`): create は「このまま作る」、edit の聞き返し中は「このまま反映」
/// - 入力はマイク(WhisperKit・録音は親の `VoiceInputView` が持つ)+ 文字入力
struct ClarifyChatView: View {
    @Environment(AppRuntime.self) private var rt
    let micMode: MicButton.Mode
    let statusText: String
    let onMic: () -> Void
    let onSend: (String) -> Void

    @State private var typed = ""
    /// #220: 複数選択中の案・チップ。返事(吹き出しの数)が変わったら空に戻す
    @State private var pick = MultiPick()
    @FocusState private var typing: Bool

    private var offer: ClarifyFlow.Offer { rt.chatTurn.map(ClarifyFlow.offer(for:)) ?? .none }
    private var ready: ChatTurn? { if case .confirm = offer { return rt.chatTurn } else { return nil } }
    private var proposals: [ChatOption] { if case .proposals(let options) = offer { return options } else { return [] } }
    private var busy: Bool { rt.chatPending || rt.isMorphing || rt.paidConfirm != nil }
    private var lastAssistantId: UUID? { rt.chatBubbles.last(where: { $0.role == .assistant })?.id }
    private var multiSelect: Bool { rt.chatTurn?.isMultiSelect == true }
    private var current: MultiPick { pick.key == rt.chatBubbles.count ? pick : MultiPick(key: rt.chatBubbles.count) }

    var body: some View {
        transcriptScroll
            .padding(.top, Theme.Spacing.sm)
            // 下のバー(選択肢・ボタン・入力欄)は safeAreaInset で固定し、キーボードと同じアニメーションで一緒に上げる(#196)
            .safeAreaInset(edge: .bottom, spacing: Theme.Spacing.sm) {
                VStack(spacing: Theme.Spacing.sm) {
                    if let card = rt.errorCard {
                        ErrorCard(presentation: card)
                            .transition(.move(edge: .bottom).combined(with: .opacity))
                    }
                    if let paid = rt.paidConfirm {
                        PaidFeatureCard(confirmation: paid)
                            .transition(.move(edge: .bottom).combined(with: .opacity))
                    } else {
                        suggestionChips
                        actionButton
                    }
                    inputRow
                }
                .padding(.bottom, Theme.Spacing.sm)
            }
            // #414: 確認カードと一緒に開く課金画面(edit は音声シートの中なので、ここから重ねる)
            .modifier(PaywallPresenter(placement: .chat))
        // 設定の「頼み方のコツ」で選んだ例を返答欄に入れる(送らない・#415)
        .onChange(of: rt.composerDraft, initial: true) { _, draft in
            guard let draft else { return }
            rt.composerDraft = nil
            typed = draft
            Task { @MainActor in
                try? await Task.sleep(for: .milliseconds(500))
                typing = true
            }
        }
        #if DEBUG
        .task {
            // ヘッドレス E2E 用: `METAMON_AUTO_FOCUS=chat` で 3 秒後に返答欄へフォーカス(キーボードの確認・#143)
            guard ProcessInfo.processInfo.environment["METAMON_AUTO_FOCUS"] == "chat" else { return }
            try? await Task.sleep(for: .seconds(3))
            typing = true
        }
        #endif
    }

    // MARK: - 吹き出し

    private var transcriptScroll: some View {
        ScrollViewReader { proxy in
            ScrollView {
                VStack(spacing: Theme.Spacing.md) {
                    // 閉じた会話と反映の区切り(#210)。今の会話の上に続けて出す
                    ForEach(rt.chatHistory) { entry in
                        HistoryRow(entry: entry)
                            .id(entry.id)
                    }
                    ForEach(rt.chatBubbles) { bubble in
                        BubbleRow(bubble: bubble, emphasized: bubble.id == lastAssistantId && rt.chatTurn?.kind == .question)
                            .id(bubble.id)
                            .transition(.move(edge: .bottom).combined(with: .opacity))
                    }
                    if rt.chatPending {
                        TypingIndicator()
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .id("pending")
                            .transition(.opacity)
                    }
                    if !proposals.isEmpty, !rt.chatPending {
                        VStack(spacing: Theme.Spacing.sm) {
                            CraneMascot(pose: .idea, size: 64).frame(maxWidth: .infinity, alignment: .leading) // #102
                            if multiSelect { MultiSelectHint() }
                            ForEach(Array(proposals.enumerated()), id: \.element.id) { index, option in
                                ProposalCard(index: index + 1, option: option, disabled: busy,
                                             selection: multiSelect ? current.options.contains(option.id) : nil) {
                                    if multiSelect { toggle(option: option.id) } else { Task { await rt.choose(option) } }
                                }
                            }
                            if multiSelect { chooseSelectedButton }
                        }
                        .padding(.leading, 32)
                        .id("proposals")
                        .transition(.move(edge: .bottom).combined(with: .opacity))
                    }
                    if let ready, !rt.chatPending {
                        Group {
                            if let brief = ready.brief, rt.chatMode == .create {
                                ReadyCard(brief: brief)
                            } else if let text = ready.transcript, !text.isEmpty {
                                EditReadyCard(instruction: text)
                            }
                        }
                        .id("ready")
                        .transition(.move(edge: .bottom).combined(with: .opacity))
                    }
                }
                .padding(.vertical, Theme.Spacing.sm)
            }
            .scrollDismissesKeyboard(.interactively)
            // 中身が伸びても下端(最新)に寄せたまま。入力欄は safeAreaInset 側なので動かない(#208)
            .defaultScrollAnchor(.bottom)
            .onChange(of: rt.chatBubbles.count) { scrollToEndAfterLayout(proxy) }
            .onChange(of: rt.chatPending) { scrollToEndAfterLayout(proxy) }
            // キーボードが上がると吹き出し欄が縮むので、最新の吹き出しを見せ直す(#143)
            .onChange(of: typing) { _, isTyping in
                guard isTyping else { return }
                Task {
                    try? await Task.sleep(for: .milliseconds(350))
                    scrollToEnd(proxy)
                }
            }
        }
    }

    /// 返事と一緒に差し込まれる案のカード・要約カードは、同じ更新では id がまだ配置されておらず scrollTo が空振りする(#208)。
    /// 配置を 1 回待ってから最新までスクロールする
    private func scrollToEndAfterLayout(_ proxy: ScrollViewProxy) {
        scrollToEnd(proxy)
        Task {
            try? await Task.sleep(for: .milliseconds(120))
            scrollToEnd(proxy)
        }
    }

    private func scrollToEnd(_ proxy: ScrollViewProxy) {
        let target: AnyHashable? = rt.chatPending ? "pending"
            : ready != nil ? "ready"
            : !proposals.isEmpty ? "proposals"
            : rt.chatBubbles.last.map { AnyHashable($0.id) } ?? rt.chatHistory.last.map { AnyHashable($0.id) }
        guard let target else { return }
        withAnimation(.easeOut(duration: 0.3)) { proxy.scrollTo(target, anchor: .bottom) }
    }

    // MARK: - 複数選択(#220)

    private func toggle(option id: String) {
        var next = current
        if next.options.contains(id) { next.options.remove(id) } else { next.options.insert(id) }
        pick = next
    }

    private func toggle(chip: String) {
        var next = current
        if let i = next.chips.firstIndex(of: chip) { next.chips.remove(at: i) } else { next.chips.append(chip) }
        pick = next
    }

    /// 選んだ案(画面の並び順)をつないで 1 回で反映する。0 件では押せない
    private var chooseSelectedButton: some View {
        let selected = proposals.filter { current.options.contains($0.id) }
        let enabled = MultiSelect.canSubmit(selectedCount: selected.count) && !busy
        return Button {
            guard let combined = MultiSelect.combined(selected) else { return }
            Task { await rt.choose(combined) }
        } label: {
            Label(L10n.tr("これでいく(\(selected.count) 件)"), systemImage: "wand.and.stars")
                .font(.headline)
                .foregroundStyle(.white)
                .frame(maxWidth: .infinity)
                .frame(height: 48)
                .background(Theme.brandGradient, in: Capsule())
        }
        .buttonStyle(.plain)
        .disabled(!enabled)
        .opacity(enabled ? 1 : 0.4)
        .accessibilityIdentifier("chat.option.chooseSelected")
    }

    // MARK: - 選択肢・ボタン

    @ViewBuilder
    private var suggestionChips: some View {
        if case .chips(let chips) = offer, !chips.isEmpty, !busy {
            let multi = multiSelect && rt.chatTurn?.kind == .question
            VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
                if multi { MultiSelectHint() }
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: Theme.Spacing.sm) {
                        ForEach(chips, id: \.self) { chip in
                            let on = multi && current.chips.contains(chip)
                            Button { if multi { toggle(chip: chip) } else { onSend(chip) } } label: {
                                HStack(spacing: 4) {
                                    if multi { Image(systemName: on ? "checkmark.circle.fill" : "circle") }
                                    Text(chip)
                                }
                                .font(.subheadline.weight(.medium))
                                .foregroundStyle(on ? AnyShapeStyle(.white) : AnyShapeStyle(.primary))
                                .padding(.horizontal, Theme.Spacing.md)
                                .padding(.vertical, Theme.Spacing.sm + 2)
                                .background(on ? AnyShapeStyle(Theme.brandGradient) : AnyShapeStyle(Theme.Colors.surface), in: Capsule())
                                .overlay(Capsule().strokeBorder(Theme.Colors.brandA.opacity(0.35)))
                            }
                            .buttonStyle(.plain)
                            .accessibilityIdentifier("chat.chip")
                            .accessibilityAddTraits(on ? .isSelected : [])
                        }
                    }
                    .padding(.horizontal, 1)
                }
                if multi {
                    let enabled = MultiSelect.canSubmit(selectedCount: current.chips.count)
                    Button { onSend(MultiSelect.joinedAnswer(current.chips)) } label: {
                        Text(L10n.tr("決定"))
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(.white)
                            .frame(maxWidth: .infinity)
                            .frame(height: 40)
                            .background(Theme.brandGradient, in: Capsule())
                    }
                    .buttonStyle(.plain)
                    .disabled(!enabled)
                    .opacity(enabled ? 1 : 0.4)
                    .accessibilityIdentifier("chat.chip.submit")
                }
            }
            .transition(.opacity)
        }
    }

    /// ready なら「これでいく」(主)。それ以外は副ボタン: create =「このまま作る」(常に)、edit の聞き返し中 =「このまま反映」。
    /// 案のカードはカードごとに「これでいく」を持つのでここには出さない。
    @ViewBuilder
    private var actionButton: some View {
        if ready != nil {
            Button { Task { await rt.buildFromChat() } } label: {
                Label(rt.chatAutoBuildPending ? ClarifyCopy.autoBuilding : ClarifyCopy.buildButton, systemImage: "wand.and.stars")
                    .font(.headline)
                    .foregroundStyle(.white)
                    .frame(maxWidth: .infinity)
                    .frame(height: 52)
                    .background(Theme.brandGradient, in: Capsule())
            }
            .buttonStyle(.plain)
            .disabled(busy)
            .accessibilityIdentifier("chat.build")
        } else if case .applyNow = ClarifyFlow.shortcut(for: rt.chatTurn, mode: rt.chatMode) {
            Button { Task { await rt.applyNow() } } label: {
                Label(ClarifyCopy.applyNowButton, systemImage: "forward.fill")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(Theme.Colors.brandA)
                    .frame(maxWidth: .infinity)
                    .frame(height: 40)
                    .background(Theme.Colors.surface, in: Capsule())
                    .overlay(Capsule().strokeBorder(Theme.Colors.brandA.opacity(0.4)))
            }
            .buttonStyle(.plain)
            .disabled(busy)
            .opacity(busy ? 0.5 : 1)
            .accessibilityIdentifier("chat.applyNow")
        } else if case .buildNow = ClarifyFlow.shortcut(for: rt.chatTurn, mode: rt.chatMode) {
            Button { Task { await rt.buildNow() } } label: {
                Label(ClarifyCopy.buildNowButton, systemImage: "forward.fill")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(Theme.Colors.brandA)
                    .frame(maxWidth: .infinity)
                    .frame(height: 40)
                    .background(Theme.Colors.surface, in: Capsule())
                    .overlay(Capsule().strokeBorder(Theme.Colors.brandA.opacity(0.4)))
            }
            .buttonStyle(.plain)
            .disabled(busy)
            .opacity(busy ? 0.5 : 1)
            .accessibilityIdentifier("chat.buildNow")
        }
    }

    // MARK: - 入力(マイク + 文字)

    /// マイク + 伸びる文字入力 + 送信(#196)。return は改行、送信はボタン。送信後もキーボードは出したまま(続けて打てる)。
    private var inputRow: some View {
        ChatComposer(placeholder: rt.chatMode == .edit ? ClarifyCopy.editReplyPlaceholder : ClarifyCopy.replyPlaceholder,
                     text: $typed, focused: $typing, disabled: busy, onSend: onSend) {
            MicButton(mode: micMode, size: 34, action: onMic)
                .frame(width: 48, height: ChatComposer<EmptyView>.fieldMinHeight)
                .accessibilityHint(statusText)
        }
        .overlay(alignment: .topLeading) {
            if micMode == .listening {
                Text(statusText)
                    .font(.caption.weight(.medium))
                    .foregroundStyle(Theme.Colors.recordA)
                    .offset(y: -18)
            }
        }
    }
}

// MARK: - Parts

/// 有料機能の確認カード(#77)。「〇〇には Tailor It Pro(¥500 / 月)が必要です」→ 購入して作る(= 課金画面・#414)/ この機能なしで作る。
private struct PaidFeatureCard: View {
    @Environment(AppRuntime.self) private var rt
    let confirmation: PaidConfirmation

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.md) {
            HStack(alignment: .top) {
                Label(L10n.tr("\(confirmation.featureNames)には Tailor It Pro(\(rt.proMonthlyPrice) / 月)が必要です"), systemImage: "crown.fill")
                    .font(.headline)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityElement(children: .combine)
                    .accessibilityIdentifier("paid.headline")
                Spacer(minLength: Theme.Spacing.sm)
                Button { rt.dismissPaidConfirm() } label: {
                    Image(systemName: "xmark.circle.fill").font(.title3).foregroundStyle(.secondary)
                }
                .buttonStyle(.plain)
                .accessibilityLabel(L10n.tr("やめる"))
                .accessibilityIdentifier("paid.cancel")
            }
            ForEach(confirmation.features) { feature in
                if !feature.reason.isEmpty {
                    Text(feature.reason)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            Button { Task { await rt.purchaseAndBuild() } } label: {
                HStack(spacing: Theme.Spacing.sm) {
                    if rt.paidConfirmBusy { ProgressView().tint(.white) }
                    Text(L10n.tr("購入して作る")).font(.headline)
                }
                .foregroundStyle(.white)
                .frame(maxWidth: .infinity)
                .frame(height: 48)
                .background(Theme.brandGradient, in: Capsule())
            }
            .buttonStyle(.plain)
            .disabled(rt.paidConfirmBusy)
            .accessibilityIdentifier("paid.purchase")
            Button { Task { await rt.buildWithoutPaidFeatures() } } label: {
                Text(L10n.tr("この機能なしで作る"))
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(Theme.Colors.brandA)
                    .frame(maxWidth: .infinity)
                    .frame(height: 40)
                    .background(Theme.Colors.surface, in: Capsule())
                    .overlay(Capsule().strokeBorder(Theme.Colors.brandA.opacity(0.4)))
            }
            .buttonStyle(.plain)
            .disabled(rt.paidConfirmBusy)
            .accessibilityIdentifier("paid.withoutFeature")
            if let message = rt.paidConfirmMessage {
                Text(message)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("paid.message")
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .metamonCard()
    }
}

private struct BubbleRow: View {
    let bubble: AppRuntime.ChatBubble
    /// 最新の質問(読みやすく大きく)
    let emphasized: Bool

    var body: some View {
        HStack {
            if bubble.role == .user { Spacer(minLength: 48) }
            if bubble.role == .assistant {
                Image(systemName: "sparkles")
                    .font(.caption.weight(.bold))
                    .foregroundStyle(Theme.brandGradient)
                    .frame(width: 24, height: 24)
                    .background(Theme.Colors.surface, in: Circle())
                    .frame(maxHeight: .infinity, alignment: .top)
            }
            VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                if bubble.confirmsPublic { PublicConfirmTag() } // #253 公開してよいかの確認
                Text(bubble.text)
                    .font(emphasized ? .title3.weight(.semibold) : .body)
                    .foregroundStyle(bubble.role == .user ? AnyShapeStyle(.white) : AnyShapeStyle(.primary))
            }
            .padding(.horizontal, Theme.Spacing.md)
            .padding(.vertical, Theme.Spacing.sm + 2)
            .background(background, in: RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
            .fixedSize(horizontal: false, vertical: true)
            if bubble.role == .assistant { Spacer(minLength: 32) }
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel((bubble.role == .user ? L10n.tr("あなた: ") : "AI: ") + (bubble.confirmsPublic ? L10n.tr("公開の記録の確認") + "。" : "") + bubble.text)
    }

    private var background: AnyShapeStyle {
        bubble.role == .user ? AnyShapeStyle(Theme.brandGradient) : AnyShapeStyle(Theme.Colors.surface)
    }
}

/// 閉じた会話の 1 件(#210)。吹き出しは今の会話と同じ見た目、反映は区切り線 +「ここで反映しました(v3)・〜」。
private struct HistoryRow: View {
    let entry: ChatHistoryEntry

    var body: some View {
        switch entry.kind {
        case .user:
            BubbleRow(bubble: .init(role: .user, text: entry.text), emphasized: false)
        case .assistant:
            BubbleRow(bubble: .init(role: .assistant, text: entry.text), emphasized: false)
        case .applied:
            HStack(spacing: Theme.Spacing.sm) {
                line
                Label(ChatContext.dividerText(entry), systemImage: "checkmark.circle.fill")
                    .font(.caption.weight(.medium))
                    .frame(maxWidth: .infinity)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
                line
            }
            .padding(.vertical, Theme.Spacing.xs)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("chat.history.applied")
        }
    }

    private var line: some View {
        Rectangle().fill(Theme.Colors.hairline).frame(width: 20, height: 1)
    }
}

/// ready の要約(brief)。「この内容で作ります」+ アプリ名・目的・記録する項目・見たいもの。
private struct ReadyCard: View {
    let brief: ChatBrief

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            Label(ClarifyCopy.readyTitle, systemImage: "checkmark.seal.fill")
                .font(.caption.weight(.semibold))
                .foregroundStyle(Theme.brandGradient)
            Text(brief.name).font(.title3.weight(.bold))
            if !brief.purpose.isEmpty {
                Text(brief.purpose).font(.subheadline).foregroundStyle(.secondary)
            }
            row(L10n.tr("記録する"), brief.entities)
            row(L10n.tr("見られる"), brief.features)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .metamonCard()
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private func row(_ label: String, _ items: [String]) -> some View {
        if !items.isEmpty {
            HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.sm) {
                Text(label).font(.caption.weight(.semibold)).foregroundStyle(.tertiary).frame(width: 52, alignment: .leading)
                Text(items.joined(separator: L10n.tr("・"))).font(.subheadline)
            }
        }
    }
}

/// #220: 複数選択中の案(id)・チップ(押した順)。key = その時の吹き出しの数
private struct MultiPick {
    var key = -1
    var options: Set<String> = []
    var chips: [String] = []
}

/// #220: 「いくつでも選べます」
private struct MultiSelectHint: View {
    var body: some View {
        Label(L10n.tr("いくつでも選べます"), systemImage: "checklist")
            .font(.caption.weight(.semibold))
            .foregroundStyle(.secondary)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// proposal の案 1 つ(#186)。タイトル・説明 +「これでいく」。押すまでアプリは変わらない。
/// #220: selection が nil でない時(複数選択)はカード全体がチェックの切り替えになり、「これでいく」は下にまとめて出す。
private struct ProposalCard: View {
    let index: Int
    let option: ChatOption
    let disabled: Bool
    var selection: Bool?
    let onChoose: () -> Void

    var body: some View {
        if let selected = selection {
            Button(action: onChoose) {
                HStack(alignment: .top, spacing: Theme.Spacing.sm) {
                    content
                    Image(systemName: selected ? "checkmark.circle.fill" : "circle")
                        .font(.title2)
                        .foregroundStyle(selected ? AnyShapeStyle(Theme.brandGradient) : AnyShapeStyle(.tertiary))
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .metamonCard()
                .overlay(RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous)
                    .strokeBorder(Theme.Colors.brandA.opacity(selected ? 0.8 : 0), lineWidth: 2))
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .disabled(disabled)
            .accessibilityElement(children: .combine)
            .accessibilityAddTraits(selected ? .isSelected : [])
            .accessibilityValue(selected ? L10n.tr("選択中") : L10n.tr("未選択"))
            .accessibilityIdentifier("chat.option.toggle")
        } else {
            single
        }
    }

    private var content: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.sm) {
                Text(L10n.tr("案\(index)"))
                    .font(.caption.weight(.bold))
                    .foregroundStyle(.white)
                    .padding(.horizontal, Theme.Spacing.sm)
                    .padding(.vertical, 2)
                    .background(Theme.brandGradient, in: Capsule())
                Text(option.title)
                    .font(.headline)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if !option.detail.isEmpty {
                Text(option.detail)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var single: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.sm) {
                Text(L10n.tr("案\(index)"))
                    .font(.caption.weight(.bold))
                    .foregroundStyle(.white)
                    .padding(.horizontal, Theme.Spacing.sm)
                    .padding(.vertical, 2)
                    .background(Theme.brandGradient, in: Capsule())
                Text(option.title)
                    .font(.headline)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if !option.detail.isEmpty {
                Text(option.detail)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Button(action: onChoose) {
                Label(ClarifyCopy.buildButton, systemImage: "wand.and.stars")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(.white)
                    .frame(maxWidth: .infinity)
                    .frame(height: 40)
                    .background(Theme.brandGradient, in: Capsule())
            }
            .buttonStyle(.plain)
            .disabled(disabled)
            .opacity(disabled ? 0.5 : 1)
            .accessibilityIdentifier("chat.option.choose")
            .accessibilityLabel(L10n.tr("これでいく: \(option.title)"))
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .metamonCard()
    }
}

/// edit の ready(#186): 「この内容で変えます」+ 変更の指示文。
private struct EditReadyCard: View {
    let instruction: String

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            Label(ClarifyCopy.editReadyTitle, systemImage: "checkmark.seal.fill")
                .font(.caption.weight(.semibold))
                .foregroundStyle(Theme.brandGradient)
            Text(instruction)
                .font(.body.weight(.medium))
                .fixedSize(horizontal: false, vertical: true)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .metamonCard()
        .accessibilityElement(children: .combine)
    }
}

/// 応答待ちの「…」+「考えています…」(案は 3〜4 秒かかるので、止まって見えないよう文言も出す・#185)。
private struct TypingIndicator: View {
    var body: some View {
        TimelineView(.animation(minimumInterval: 1.0 / 20)) { tl in
            let t = tl.date.timeIntervalSinceReferenceDate
            HStack(spacing: Theme.Spacing.sm) {
                CraneMascot(pose: .tea, size: 44, sway: 0) // #102
                dots(t)
                Text(ClarifyCopy.thinking)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            .padding(.horizontal, Theme.Spacing.md)
            .padding(.vertical, Theme.Spacing.sm + 4)
            .background(Theme.Colors.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
            .padding(.leading, 32)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(ClarifyCopy.thinking)
        .accessibilityIdentifier("chat.pending")
    }

    private func dots(_ t: TimeInterval) -> some View {
            HStack(spacing: 5) {
                ForEach(0..<3) { i in
                    Circle()
                        .fill(Color.secondary)
                        .frame(width: 7, height: 7)
                        .opacity(0.35 + 0.65 * max(0, sin(t * 5 - Double(i) * 0.7)))
                }
            }
    }
}
