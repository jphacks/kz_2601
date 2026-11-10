import SwiftUI
import MetamonDSL
import MetamonServices

/// 作る・直す会話のシート(#967)。中央の列の上に出すので、直す時は後ろのアプリが見えたまま話せる。
/// iOS の ClarifyChatView(吹き出し・チップ・案・要約カード)・GeneratingView(生成中)・組み上げを 1 枚にまとめた。
/// キーボード: 入力欄で Return = 送る・Esc = 閉じる・「これでいく」は ⌘Return。チップと案は Tab で回ってスペースで押せる。
/// 声で頼む(#966): 入力欄の横のマイク・⇧⌘M で録音 → もう一度で止めて文字にし、そのまま送る。
struct ComposeSheet: View {
    @Bindable var session: MacComposeSession
    var onClose: () -> Void
    @Environment(MacVoiceInput.self) private var voice

    static let size = CGSize(width: 560, height: 620)

    var body: some View {
        VStack(spacing: 0) {
            header
            Divider()
            switch session.phase {
            case .chatting:
                ComposeChat(session: session)
            case .generating(let job):
                ComposeGenerating(session: session, job: job, plan: nil)
            case .revealing(let job, let plan):
                ComposeGenerating(session: session, job: job, plan: plan)
            case .finished(let summary, let unchanged):
                ComposeFinished(summary: summary, unchanged: unchanged, onClose: close)
            }
        }
        .frame(minWidth: Self.size.width, idealWidth: Self.size.width, minHeight: Self.size.height, idealHeight: Self.size.height)
        .background(.background)
        .modifier(MacModelDownloadConfirm())
        .onDisappear { voice.cancel() } // 録音中に閉じたら録音を捨てる
    }

    private var title: LocalizedStringKey {
        switch session.target {
        case .create: L10n.key("新しいアプリを作る", review: "新しい記録帳を用意する")
        case .edit: L10n.key("このアプリを直す", review: "この記録帳を直す")
        }
    }

    private var header: some View {
        HStack(spacing: MacTheme.Spacing.group) {
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(.headline)
                if case .edit(_, let name, _) = session.target {
                    Text(name).font(.subheadline).foregroundStyle(.secondary).lineLimit(1)
                }
            }
            Spacer()
            Button(action: close) { Image(systemName: "xmark.circle.fill").font(.title3) }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)
                .keyboardShortcut(.cancelAction)
                .help("閉じる")
                .accessibilityLabel("閉じる")
        }
        .padding(.horizontal, MacTheme.Spacing.window)
        .padding(.vertical, MacTheme.Spacing.group)
    }

    private func close() {
        voice.cancel()
        session.close()
        onClose()
    }
}

// MARK: - 会話

private struct ComposeChat: View {
    @Bindable var session: MacComposeSession
    @Environment(MacVoiceInput.self) private var voice
    @FocusState private var inputFocused: Bool
    @State private var showsTips = false

    var body: some View {
        VStack(spacing: 0) {
            ScrollViewReader { proxy in
                ScrollView {
                    VStack(alignment: .leading, spacing: MacTheme.Spacing.group) {
                        if session.bubbles.isEmpty { greeting }
                        ForEach(session.bubbles) { bubble in
                            ChatBubbleRow(bubble: bubble).id(bubble.id)
                        }
                        if session.sending {
                            ProgressView().controlSize(.small).padding(.leading, 4)
                                .accessibilityLabel("考えています")
                        }
                        if !session.sending { offerView }
                        if let error = session.error {
                            Label(error, systemImage: "exclamationmark.triangle")
                                .font(.callout).foregroundStyle(Theme.Colors.warning)
                        }
                        Color.clear.frame(height: 1).id("bottom")
                    }
                    .padding(MacTheme.Spacing.window)
                }
                .onChange(of: session.bubbles.count) {
                    withAnimation { proxy.scrollTo("bottom", anchor: .bottom) }
                }
                .onChange(of: session.sending) {
                    withAnimation { proxy.scrollTo("bottom", anchor: .bottom) }
                }
            }
            Divider()
            MacVoiceStatus()
            inputBar
        }
        .onAppear {
            inputFocused = true
            // ⇧⌘M(声で頼む)で開いた時はすぐ録音を始める
            if session.startsWithVoice {
                session.startsWithVoice = false
                voice.toggle(deliver: deliver)
            }
        }
    }

    /// 文字にした文を(書きかけがあればその後ろに足して)送る。送っている途中なら入力欄に足すだけ(送るのは本人が Return)
    private func deliver(_ text: String) {
        let typed = session.draft.trimmingCharacters(in: .whitespacesAndNewlines)
        session.draft = typed.isEmpty ? text : typed + " " + text
        if session.phase == .chatting, !session.sending { session.sendDraft() }
    }

    private var greeting: some View {
        HStack(alignment: .top, spacing: MacTheme.Spacing.group) {
            CraneMascot(pose: .happy, size: 56)
            VStack(alignment: .leading, spacing: 4) {
                switch session.target {
                case .create:
                    Text(L10n.key("どんなアプリを作りますか?", review: "どんな記録帳を用意しますか?")).font(.title3.weight(.semibold))
                    Text("「毎日の支出を記録して、今月の合計を見たい」のように、使い道を書いてください。")
                        .foregroundStyle(.secondary)
                case .edit:
                    Text("どこをどう直しますか?").font(.title3.weight(.semibold))
                    Text("「合計をもっと大きく表示して」のように、変えたい所を書いてください。")
                        .foregroundStyle(.secondary)
                }
            }
        }
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder private var offerView: some View {
        VStack(alignment: .leading, spacing: MacTheme.Spacing.group) {
            switch session.offer {
            case .none:
                EmptyView()
            case .chips(let chips):
                ChipsView(session: session, chips: chips)
            case .proposals(let options):
                ProposalsView(session: session, options: options)
            case .confirm:
                ConfirmCard(session: session)
            }
            switch session.shortcut {
            case .none: EmptyView()
            case .buildNow:
                Button(L10n.key("このまま作る", review: "このまま用意する")) { session.runShortcut() }
                    .help("聞き返しを飛ばして、ここまでの内容で作ります")
            case .applyNow:
                Button("このまま反映") { session.runShortcut() }
                    .help("ここまでの指示でアプリを直します")
            }
        }
    }

    private var inputBar: some View {
        HStack(spacing: 8) {
            TextField(session.bubbles.isEmpty ? "ここに書いて Return で送る" : "答えや続きを書く",
                      text: $session.draft, axis: .vertical)
                .textFieldStyle(.plain)
                .lineLimit(1...4)
                .focused($inputFocused)
                .onSubmit { session.sendDraft() }
                .disabled(session.sending || voice.isBusy)
            MacMicButton(disabled: session.sending, deliver: deliver)
            Button {
                showsTips = true
            } label: {
                Image(systemName: "lightbulb")
            }
            .buttonStyle(.borderless)
            .help(PromptTips.title)
            .accessibilityLabel(Text(PromptTips.title))
            .popover(isPresented: $showsTips, arrowEdge: .top) {
                PromptTipsList { tip in
                    session.draft = tip
                    showsTips = false
                    inputFocused = true
                }
                .frame(width: 420, height: 440)
            }
            Button {
                session.sendDraft()
            } label: {
                Image(systemName: "arrow.up.circle.fill").font(.title2)
            }
            .buttonStyle(.borderless)
            .disabled(session.sending || session.draft.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            .help("送る")
            .accessibilityLabel("送る")
        }
        .padding(.horizontal, MacTheme.Spacing.window)
        .padding(.vertical, MacTheme.Spacing.group)
    }
}

private struct ChatBubbleRow: View {
    let bubble: MacComposeSession.Bubble

    private var isUser: Bool { bubble.role == .user }

    var body: some View {
        HStack {
            if isUser { Spacer(minLength: 60) }
            Text(bubble.text)
                .textSelection(.enabled)
                .padding(.horizontal, 12).padding(.vertical, 8)
                .foregroundStyle(isUser ? AnyShapeStyle(.white) : AnyShapeStyle(.primary))
                .background(isUser ? AnyShapeStyle(Theme.brandGradient) : AnyShapeStyle(.quaternary),
                            in: RoundedRectangle(cornerRadius: 14, style: .continuous))
            if !isUser { Spacer(minLength: 60) }
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(isUser ? Text("あなた: \(bubble.text)") : Text(bubble.text))
    }
}

/// 聞き返しの選択肢。複数選択なら押して選び「決定」で送る。
private struct ChipsView: View {
    @Bindable var session: MacComposeSession
    let chips: [String]

    private var multi: Bool { session.latest?.isMultiSelect == true && session.latest?.kind == .question }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if multi {
                Text("当てはまるものを全部選んでください").font(.caption).foregroundStyle(.secondary)
            }
            HStack(spacing: 8) {
                ForEach(chips, id: \.self) { chip in
                    let selected = session.selectedChips.contains(chip)
                    Button { session.tapChip(chip) } label: {
                        Label(chip, systemImage: multi ? (selected ? "checkmark.circle.fill" : "circle") : "arrow.turn.down.right")
                    }
                    .buttonStyle(.bordered)
                    .tint(selected ? .accentColor : nil)
                    .accessibilityAddTraits(selected ? .isSelected : [])
                }
            }
            if multi {
                Button("決定") { session.submitChips() }
                    .buttonStyle(.borderedProminent)
                    .keyboardShortcut(.return, modifiers: .command)
                    .disabled(!MultiSelect.canSubmit(selectedCount: session.selectedChips.count))
            }
        }
    }
}

/// 案のカード。複数選択なら選んで「これでいく(N 件)」、1 つずつなら各カードの「これでいく」。
private struct ProposalsView: View {
    @Bindable var session: MacComposeSession
    let options: [ChatOption]

    private var multi: Bool { session.latest?.isMultiSelect == true }
    private var chosen: [ChatOption] { options.filter { session.selectedOptions.contains($0.id) } }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            ForEach(options) { option in
                let selected = session.selectedOptions.contains(option.id)
                HStack(alignment: .top, spacing: MacTheme.Spacing.group) {
                    if multi {
                        Toggle(isOn: Binding(get: { selected }, set: { _ in session.toggleOption(option) })) { EmptyView() }
                            .toggleStyle(.checkbox)
                            .labelsHidden()
                            .accessibilityLabel(Text(option.title))
                    }
                    VStack(alignment: .leading, spacing: 4) {
                        Text(option.title).font(.headline)
                        if !option.detail.isEmpty {
                            Text(option.detail).font(.callout).foregroundStyle(.secondary)
                        }
                        if option.requiresPro == true {
                            PaidNote()
                        }
                    }
                    Spacer()
                    if !multi {
                        Button("これでいく") { session.choose([option]) }
                            .buttonStyle(.borderedProminent)
                    }
                }
                .padding(MacTheme.Spacing.group)
                .background(.quaternary.opacity(0.5), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
            }
            if multi {
                Button("これでいく(\(chosen.count) 件)") { session.choose(chosen) }
                    .buttonStyle(.borderedProminent)
                    .keyboardShortcut(.return, modifiers: .command)
                    .disabled(!MultiSelect.canSubmit(selectedCount: chosen.count))
            }
        }
    }
}

/// ready の要約カード(何を作るか)と「これでいく」。
private struct ConfirmCard: View {
    @Bindable var session: MacComposeSession

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let brief = session.latest?.brief {
                Text(brief.name).font(.headline)
                if !brief.purpose.isEmpty { Text(brief.purpose).font(.callout) }
                ForEach(brief.features, id: \.self) { feature in
                    Label(feature, systemImage: "checkmark").font(.callout).foregroundStyle(.secondary)
                }
            } else if case .confirm(let transcript?) = session.offer {
                Text(transcript).font(.callout)
            }
            if session.dropsPaidFeatures { PaidNote() }
            Button {
                session.confirm()
            } label: {
                Text("これでいく").frame(minWidth: 120)
            }
            .buttonStyle(.borderedProminent)
            .keyboardShortcut(.return, modifiers: .command)
            .help("⌘Return")
        }
        .padding(MacTheme.Spacing.group)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.quaternary.opacity(0.5), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
    }
}

/// 有料機能は Mac ではまだ使えない(課金は後の Issue)。無料版で作ることを先に言う。
private struct PaidNote: View {
    var body: some View {
        Label("有料の機能は Mac ではまだ使えないため、無料版で作ります", systemImage: "info.circle")
            .font(.caption)
            .foregroundStyle(.secondary)
    }
}

// MARK: - 生成中・組み上げ

private struct ComposeGenerating: View {
    let session: MacComposeSession
    let job: MacComposeSession.Job
    let plan: BuildChoreography.AssemblyPlan?
    @Environment(\.colorScheme) private var scheme

    private var edit: Bool { session.mode == .edit }

    var body: some View {
        TimelineView(.periodic(from: job.started, by: 0.25)) { context in
            let elapsed = max(0, context.date.timeIntervalSince(job.started))
            let stage = GeneratingStage(elapsed: elapsed)
            VStack(spacing: MacTheme.Spacing.group) {
                Text(job.transcript)
                    .font(.title3.weight(.semibold))
                    .multilineTextAlignment(.center)
                    .lineLimit(3)
                BuildStageView(script: job.script, elapsed: elapsed, plan: plan, live: job.live)
                    .frame(maxWidth: 300)
                    .contentShape(Rectangle())
                    .onTapGesture { session.skipReveal() }
                if plan == nil {
                    HStack(spacing: 8) {
                        ProgressView().controlSize(.small)
                        Text(stage.message(elapsed: elapsed, edit: edit))
                            .foregroundStyle(stage == .normal || stage == .slow ? AnyShapeStyle(.secondary) : AnyShapeStyle(Theme.Colors.warning))
                        Text(verbatim: "\(Int(elapsed))s").monospacedDigit().foregroundStyle(.tertiary)
                    }
                    .font(.callout)
                    .accessibilityElement(children: .combine)
                    if let estimate = stage.estimate(edit: edit) {
                        Text(estimate).font(.caption).foregroundStyle(.secondary)
                    }
                    Button("やめる") { session.cancelBuild() }
                        .buttonStyle(stage.cancelIsProminent ? AnyButtonStyle(.bordered) : AnyButtonStyle(.link))
                } else {
                    Button(L10n.key("できあがりました", review: "用意できました")) { session.skipReveal() }
                        .buttonStyle(.link)
                        .keyboardShortcut(.defaultAction)
                }
            }
            .padding(MacTheme.Spacing.window)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
        .background { Theme.backdrop(scheme) }
    }
}

/// `.buttonStyle` を段階で切り替える(bordered ↔ link)
private struct AnyButtonStyle: PrimitiveButtonStyle {
    private let make: (Configuration) -> AnyView

    init<S: PrimitiveButtonStyle>(_ style: S) {
        make = { config in AnyView(Button(role: config.role, action: config.trigger) { config.label }.buttonStyle(style)) }
    }

    func makeBody(configuration: Configuration) -> some View { make(configuration) }
}

// MARK: - 完成

private struct ComposeFinished: View {
    let summary: String
    let unchanged: Bool
    let onClose: () -> Void

    var body: some View {
        VStack(spacing: MacTheme.Spacing.group) {
            CraneMascot(pose: unchanged ? .panic : .joy, size: MacTheme.Mascot.small)
            Text(unchanged ? "変えられませんでした" : L10n.key("できあがりました", review: "用意できました"))
                .font(MacTheme.Fonts.emptyTitle)
            Text(summary)
                .multilineTextAlignment(.center)
                .textSelection(.enabled)
                .foregroundStyle(.secondary)
            Button {
                onClose()
            } label: {
                Text(unchanged ? "閉じる" : "アプリを見る").frame(minWidth: 120)
            }
            .buttonStyle(.borderedProminent)
            .keyboardShortcut(.defaultAction)
        }
        .padding(MacTheme.Spacing.window)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityElement(children: .contain)
    }
}
