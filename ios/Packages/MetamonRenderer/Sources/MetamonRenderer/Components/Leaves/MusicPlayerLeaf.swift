import SwiftUI
import MetamonDSL

/// `musicPlayer`(#412): Apple Music の再生(今の曲・前へ / 再生・一時停止 / 次へ・再生待ち)と、友だちから届いたリクエスト。
/// 状態は `MusicSession.shared`(端末に 1 つのプレーヤー)。前面にいる間は 1 秒ごとに読み直し、ロック画面・イヤホンでの操作も映す。
/// 友だちへのリクエスト送信は近距離共有(`DSLRuntime.requestSong`)。審査用ビルド(APP_STORE_REVIEW)ではリクエストの UI を外す。
struct MusicPlayerLeaf: View {
    let c: Component
    @Environment(\.dslRuntime) private var runtime
    @Environment(\.scenePhase) private var scenePhase
    @State private var session = MusicSession.shared
    @State private var searching = false
    /// 検索シートで選んだリクエスト。シートを閉じてから送る(シートの上に送信シートは重ねられない)
    @State private var outgoing: MusicSong?

    /// 友だちからのリクエストを受ける・送れるか(審査用ビルドでは常に false)
    private var requests: Bool {
        #if APP_STORE_REVIEW
        false
        #else
        c.allowRequests == true
        #endif
    }

    var body: some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.md) {
            header
            status
            nowPlaying
            controls
            if let notice = session.notice {
                Text(notice)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
                    .accessibilityIdentifier("music.notice")
            }
            lists
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .renderCard(padding: RenderTheme.Spacing.md, wash: true)
        .task(id: scenePhase == .active) {
            guard scenePhase == .active else { return }
            await session.prepare(requesting: false)
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(1))
                session.refresh()
            }
        }
        .sheet(isPresented: $searching, onDismiss: sendOutgoing) {
            MusicSearchSheet(session: session, requests: requests) { outgoing = $0 }
        }
    }

    private var header: some View {
        HStack(spacing: 6) {
            Image(systemName: "music.note")
                .font(.caption.weight(.bold))
                .foregroundStyle(RenderTheme.Colors.brandA)
                .frame(width: 18, height: 18)
                .background(RenderTheme.Colors.brandSoft, in: RoundedRectangle(cornerRadius: 6, style: .continuous))
                .accessibilityHidden(true)
            Text(c.title ?? L("音楽"))
                .font(RenderTheme.Fonts.statLabel)
                .foregroundStyle(.secondary)
                .lineLimit(1)
            Spacer(minLength: 8)
            Button {
                searching = true
            } label: {
                Label(L("曲を探す"), systemImage: "magnifyingglass")
                    .font(.subheadline.weight(.semibold))
            }
            .accessibilityIdentifier("music.search")
        }
    }

    @ViewBuilder
    private var status: some View {
        switch session.availability {
        case .notDetermined:
            Button {
                Task { await session.prepare(requesting: true) }
            } label: {
                Label(L("Apple Music を使う"), systemImage: "music.note.list")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(RenderButtonStyle(kind: .secondary))
        case .ready:
            EmptyView()
        default:
            if let message = session.availability.message {
                Label(message, systemImage: "exclamationmark.triangle")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    private var nowPlaying: some View {
        HStack(spacing: RenderTheme.Spacing.sm) {
            MusicArtwork(url: session.state.nowPlaying?.artworkURL, size: 56)
            VStack(alignment: .leading, spacing: 2) {
                if let song = session.state.nowPlaying {
                    Text(song.title).font(.headline).lineLimit(1)
                    Text(song.artist).font(.subheadline).foregroundStyle(.secondary).lineLimit(1)
                } else {
                    Text(L("再生待ちは空です")).font(.headline)
                    Text(L("「曲を探す」から追加できます")).font(.caption).foregroundStyle(.secondary)
                }
            }
            Spacer(minLength: 0)
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("music.nowPlaying")
    }

    private var controls: some View {
        HStack {
            Spacer()
            controlButton("backward.fill", L("前の曲")) { await session.skipToPrevious() }
            Spacer()
            controlButton(session.state.isPlaying ? "pause.fill" : "play.fill",
                          session.state.isPlaying ? L("一時停止") : L("再生"), size: 30) { await session.togglePlay() }
                .accessibilityIdentifier("music.playPause")
            Spacer()
            controlButton("forward.fill", L("次の曲")) { await session.skipToNext() }
            Spacer()
        }
        .disabled(session.state.nowPlaying == nil || !session.availability.canPlay)
    }

    private func controlButton(_ icon: String, _ label: String, size: CGFloat = 22, _ op: @escaping () async -> Void) -> some View {
        Button {
            Task { await op() }
        } label: {
            Image(systemName: icon)
                .font(.system(size: size, weight: .semibold))
                .frame(width: 52, height: 44)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .foregroundStyle(RenderTheme.Colors.brandA)
        .accessibilityLabel(label)
    }

    private var lists: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
                if requests && !session.inbox.pending.isEmpty {
                    sectionTitle(L("リクエスト(\(session.inbox.pending.count))"))
                    ForEach(session.inbox.pending) { request in
                        MusicRequestRow(request: request,
                                        canAdd: session.availability.canPlay,
                                        accept: { Task { await session.accept(request.id) } },
                                        reject: { session.reject(request.id) })
                    }
                }
                if !session.state.upNext.isEmpty {
                    sectionTitle(L("再生待ち"))
                    ForEach(Array(session.state.upNext.enumerated()), id: \.offset) { _, song in
                        MusicSongRow(song: song)
                    }
                }
                if requests {
                    Text(L("近くの友だちが同じアプリから曲をリクエストできます(届いたら「追加」で再生待ちへ)"))
                        .font(.caption2)
                        .foregroundStyle(.tertiary)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
        }
    }

    private func sectionTitle(_ text: String) -> some View {
        Text(text)
            .font(.caption.weight(.semibold))
            .foregroundStyle(.secondary)
            .padding(.top, 4)
    }

    private func sendOutgoing() {
        guard let song = outgoing else { return }
        outgoing = nil
        runtime.requestSong(song)
    }
}

/// ジャケット(無ければ音符)。
struct MusicArtwork: View {
    let url: String?
    var size: CGFloat = 40

    var body: some View {
        let shape = RoundedRectangle(cornerRadius: size * 0.18, style: .continuous)
        AsyncImage(url: url.flatMap(URL.init(string:))) { phase in
            if let image = phase.image {
                image.resizable().scaledToFill()
            } else {
                Image(systemName: "music.note")
                    .font(.system(size: size * 0.4, weight: .semibold))
                    .foregroundStyle(RenderTheme.Colors.brandA)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .background(RenderTheme.Colors.brandSoft)
            }
        }
        .frame(width: size, height: size)
        .clipShape(shape)
        .accessibilityHidden(true)
    }
}

struct MusicSongRow: View {
    let song: MusicSong

    var body: some View {
        HStack(spacing: RenderTheme.Spacing.sm) {
            MusicArtwork(url: song.artworkURL, size: 36)
            VStack(alignment: .leading, spacing: 1) {
                Text(song.title).font(.subheadline).lineLimit(1)
                if !song.artist.isEmpty {
                    Text(song.artist).font(.caption).foregroundStyle(.secondary).lineLimit(1)
                }
            }
            Spacer(minLength: 0)
        }
        .accessibilityElement(children: .combine)
    }
}

/// 届いたリクエスト 1 件: 追加 / 見送る と、Spotify で探す(開くだけ・操作はできない)。
struct MusicRequestRow: View {
    let request: MusicRequest
    let canAdd: Bool
    let accept: () -> Void
    let reject: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: RenderTheme.Spacing.sm) {
                MusicArtwork(url: request.song.artworkURL, size: 36)
                VStack(alignment: .leading, spacing: 1) {
                    Text(request.song.title).font(.subheadline.weight(.semibold)).lineLimit(1)
                    Text(L("\(request.sender)さんから") + (request.song.artist.isEmpty ? "" : " ・ \(request.song.artist)"))
                        .font(.caption).foregroundStyle(.secondary).lineLimit(1)
                }
                Spacer(minLength: 0)
            }
            HStack(spacing: RenderTheme.Spacing.sm) {
                Button(L("追加"), action: accept)
                    .buttonStyle(.borderedProminent)
                    .disabled(!canAdd)
                    .accessibilityIdentifier("music.request.accept")
                Button(L("見送る"), role: .cancel, action: reject)
                    .buttonStyle(.bordered)
                    .accessibilityIdentifier("music.request.reject")
                Spacer(minLength: 0)
                if let url = MusicLinks.spotifySearchURL(request.song) {
                    Link(destination: url) {
                        Label(L("Spotify で探す"), systemImage: "arrow.up.right.square")
                            .font(.caption)
                    }
                }
            }
            .controlSize(.small)
        }
        .padding(10)
        .background(.background.opacity(0.6), in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}

/// 曲を探すシート: Apple Music のカタログを検索して再生待ちに入れる / 近くの人にリクエストする。
/// Apple Music を使えない人も、曲名とアーティストを打ってリクエストを送れる(受けた側が Apple Music で探す)。
struct MusicSearchSheet: View {
    let session: MusicSession
    let requests: Bool
    let onRequest: (MusicSong) -> Void
    @Environment(\.dismiss) private var dismiss
    @State private var term = ""
    @State private var results: [MusicSong] = []
    @State private var message: String?
    @State private var loading = false
    @State private var title = ""
    @State private var artist = ""

    var body: some View {
        NavigationStack {
            List {
                Section {
                    if loading {
                        ProgressView()
                    } else if let message {
                        Text(message).font(.footnote).foregroundStyle(.secondary)
                    }
                    ForEach(Array(results.enumerated()), id: \.offset) { _, song in
                        HStack {
                            MusicSongRow(song: song)
                            if session.availability.canPlay {
                                Button {
                                    Task {
                                        if await session.add(song) { dismiss() }
                                    }
                                } label: {
                                    Image(systemName: "plus.circle.fill").font(.title3)
                                }
                                .buttonStyle(.borderless)
                                .accessibilityLabel(L("再生待ちに追加"))
                            }
                            if requests {
                                Button {
                                    request(song)
                                } label: {
                                    Image(systemName: "paperplane.fill").font(.title3)
                                }
                                .buttonStyle(.borderless)
                                .accessibilityLabel(L("近くの人にリクエスト"))
                            }
                        }
                    }
                } footer: {
                    if !term.isEmpty, let url = MusicLinks.spotifySearchURL(MusicSong(title: term, artist: "")) {
                        Link(L("Spotify で「\(term)」を開く(Spotify の操作はできません)"), destination: url)
                            .font(.footnote)
                    }
                }
                if requests {
                    Section {
                        TextField(L("曲名"), text: $title)
                            .accessibilityIdentifier("music.request.title")
                        TextField(L("アーティスト(任意)"), text: $artist)
                        Button(L("近くの人にリクエスト")) {
                            request(MusicSong(title: title, artist: artist))
                        }
                        .disabled(title.trimmingCharacters(in: .whitespaces).isEmpty)
                        .accessibilityIdentifier("music.request.send")
                    } header: {
                        Text(L("曲名でリクエスト"))
                    } footer: {
                        Text(L("Apple Music を使っていなくても送れます。相手の端末が Apple Music で探して再生します。"))
                    }
                }
            }
            .searchable(text: $term, prompt: L("曲名・アーティスト"))
            .onSubmit(of: .search) { Task { await runSearch() } }
            .navigationTitle(L("曲を探す"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button(L("閉じる")) { dismiss() }
                }
            }
            .task {
                if session.availability == .notDetermined { await session.prepare(requesting: true) }
                if let m = session.availability.message, !session.availability.canSearch { message = m }
            }
        }
        .presentationDetents([.medium, .large])
    }

    private func runSearch() async {
        let t = term.trimmingCharacters(in: .whitespaces)
        guard !t.isEmpty, session.availability.canSearch else { return }
        loading = true
        defer { loading = false }
        switch await session.search(t) {
        case .success(let songs):
            results = songs
            message = songs.isEmpty ? L("見つかりませんでした") : nil
        case .failure(let e):
            results = []
            message = e.errorDescription
        }
    }

    private func request(_ song: MusicSong) {
        guard MusicRequestRules.sanitize(song) != nil else { return }
        onRequest(song)
        dismiss()
    }
}
