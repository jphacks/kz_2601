// 審査用ビルド(APP_STORE_REVIEW)は通話(#447)を外す。
#if !APP_STORE_REVIEW
import SwiftUI
import UIKit
import MetamonRenderer
import MetamonServices

/// 通話の画面(#447): 参加者のタイル・ミュート・カメラ・退出。開いたら入り、閉じたら出る。
struct CallView: View {
    let title: String
    let session: CallSession
    let close: () -> Void

    var body: some View {
        VStack(spacing: RenderTheme.Spacing.md) {
            Text(title)
                .font(.title3.weight(.bold))
                .accessibilityAddTraits(.isHeader)
            switch session.phase {
            case .idle, .joining:
                Spacer()
                ProgressView("つないでいます…")
                Spacer()
            case .connected:
                tiles
                controls
            case .ended(let reason):
                Spacer()
                ended(reason)
                Spacer()
            }
        }
        .padding()
        .background(Color(.systemBackground))
        .task { await session.join() }
        .onDisappear { Task { await session.leave() } }
    }

    private var tiles: some View {
        ScrollView {
            LazyVGrid(columns: [GridItem(.adaptive(minimum: 150), spacing: RenderTheme.Spacing.sm)], spacing: RenderTheme.Spacing.sm) {
                ForEach(session.participants) { p in
                    CallTile(participant: p, video: p.cameraOn ? session.room.videoView(for: p.id) as? UIView : nil)
                }
            }
        }
    }

    private var controls: some View {
        HStack(spacing: RenderTheme.Spacing.lg) {
            CallControl(systemImage: session.micOn ? "mic.fill" : "mic.slash.fill",
                        label: session.micOn ? "ミュート" : "ミュートを解除") { Task { await session.toggleMic() } }
                .accessibilityIdentifier("call.mic")
            if session.videoGranted {
                CallControl(systemImage: session.cameraOn ? "video.fill" : "video.slash.fill",
                            label: session.cameraOn ? "カメラを止める" : "カメラを使う") { Task { await session.toggleCamera() } }
                    .accessibilityIdentifier("call.camera")
            }
            CallControl(systemImage: "phone.down.fill", label: "退出", tint: .red) {
                Task {
                    await session.leave()
                    close()
                }
            }
            .accessibilityIdentifier("call.leave")
        }
        .padding(.bottom)
    }

    @ViewBuilder
    private func ended(_ reason: CallSession.EndReason) -> some View {
        VStack(spacing: RenderTheme.Spacing.md) {
            Text(Self.message(reason))
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("call.ended")
            if case .coins(let required, let balance) = reason, let open = CallUI.shared.openCoinShop {
                Button("コインを買う") {
                    close()
                    open(required, balance)
                }
                .buttonStyle(.borderedProminent)
            }
            Button("閉じる", action: close)
                .buttonStyle(.bordered)
                .accessibilityIdentifier("call.close")
        }
    }

    static func message(_ reason: CallSession.EndReason) -> String {
        switch reason {
        case .left: L10n.tr("通話を終えました")
        case .coins: L10n.tr("コインが足りないため、通話を続けられません")
        case .denied(.disabled): L10n.tr("通話は今使えません")
        case .denied(.notMember): L10n.tr("このアプリに参加している人だけが通話できます")
        case .denied(.rateLimited): L10n.tr("しばらくしてからもう一度お試しください")
        case .denied: L10n.tr("この通話には入れません")
        case .failed: L10n.tr("通話につながりませんでした")
        }
    }
}

/// 参加者 1 人のタイル(映像があれば映像、無ければ名前の頭文字)。
private struct CallTile: View {
    let participant: CallParticipant
    let video: UIView?

    var body: some View {
        ZStack(alignment: .bottomLeading) {
            RoundedRectangle(cornerRadius: 12)
                .fill(Color(.secondarySystemBackground))
            if let video {
                CallVideo(view: video)
                    .clipShape(RoundedRectangle(cornerRadius: 12))
            } else {
                Text(initial)
                    .font(.largeTitle.weight(.semibold))
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
            HStack(spacing: 4) {
                if !participant.micOn { Image(systemName: "mic.slash.fill") }
                Text(displayName).lineLimit(1)
            }
            .font(.caption)
            .padding(6)
            .background(.ultraThinMaterial, in: Capsule())
            .padding(6)
        }
        .aspectRatio(3 / 4, contentMode: .fit)
        .overlay(RoundedRectangle(cornerRadius: 12).stroke(participant.isSpeaking ? RenderTheme.Colors.brandA : .clear, lineWidth: 3))
        .accessibilityElement(children: .combine)
    }

    private var displayName: String {
        if participant.isLocal { return L10n.tr("自分") }
        return participant.name.isEmpty ? L10n.tr("匿名") : participant.name
    }

    private var initial: String { String(displayName.prefix(1)) }
}

private struct CallVideo: UIViewRepresentable {
    let view: UIView
    func makeUIView(context: Context) -> UIView { view }
    func updateUIView(_ uiView: UIView, context: Context) {}
}

private struct CallControl: View {
    let systemImage: String
    let label: LocalizedStringKey
    var tint: Color = .primary
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: systemImage)
                .font(.title2)
                .frame(width: 60, height: 60)
                .background(Circle().fill(Color(.secondarySystemBackground)))
                .foregroundStyle(tint)
        }
        .accessibilityLabel(Text(label))
    }
}
#endif
