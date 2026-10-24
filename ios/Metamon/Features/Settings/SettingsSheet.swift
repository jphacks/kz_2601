import SwiftUI
import MetamonServices

/// 歯車から開く設定シート。アカウント・音声モデル・接続先・アプリのリセット。
struct SettingsSheet: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(AuthSession.self) private var session
    @Environment(\.services) private var services
    @Environment(\.dismiss) private var dismiss
    @State private var confirmReset = false

    var body: some View {
        NavigationStack {
            List {
                accountSection
                voiceModelSection
                appSection
                connectionSection
            }
            .navigationTitle("設定")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) { Button("閉じる") { dismiss() } }
            }
            .confirmationDialog("アプリを初期状態に戻しますか？", isPresented: $confirmReset, titleVisibility: .visible) {
                Button("初期状態に戻す", role: .destructive) {
                    rt.resetToInitial()
                    dismiss()
                }
                Button("キャンセル", role: .cancel) {}
            } message: {
                Text("生成したアプリは音声画面だけの状態に戻ります。サーバー上のデータは削除されません。")
            }
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
    }

    private var accountSection: some View {
        Section("アカウント") {
            if let user = session.user {
                HStack(spacing: Theme.Spacing.md) {
                    Image(systemName: user.isAnonymous ? "person.crop.circle.dashed" : "apple.logo")
                        .font(.title2)
                        .foregroundStyle(Theme.brandGradient)
                        .frame(width: 36)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(user.isAnonymous ? "匿名ユーザー" : "Apple でサインイン済み")
                            .font(.body.weight(.medium))
                        Text("uid \(user.uid)")
                            .font(.caption2.monospaced())
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                            .truncationMode(.middle)
                            .textSelection(.enabled)
                    }
                }
                Button("ログアウト", role: .destructive) {
                    session.signOut()
                    dismiss()
                }
            } else {
                Text("サインインしていません").foregroundStyle(.secondary)
            }
        }
    }

    private var voiceModelSection: some View {
        Section("音声モデル(オンデバイス)") {
            LabeledContent("モデル", value: BuildConfig.whisperModel)
            LabeledContent("状態") {
                HStack(spacing: Theme.Spacing.sm) {
                    Circle().fill(statusColor).frame(width: 8, height: 8)
                    Text(statusText)
                }
            }
            if case .failed = services.transcriber.status {
                Button("再試行") { Task { try? await services.transcriber.prepare() } }
            }
        }
    }

    private var appSection: some View {
        Section {
            Button(role: .destructive) { confirmReset = true } label: {
                Label("アプリをリセット(初期状態に戻す)", systemImage: "arrow.counterclockwise")
            }
            .disabled(rt.isMorphing)
        } header: {
            Text("アプリ")
        } footer: {
            Text("現在: \(rt.dsl.app.name)(v\(rt.document.version))")
        }
    }

    private var connectionSection: some View {
        Section("接続先") {
            LabeledContent("ai", value: services.ai is MockAIClient ? "モック" : (BuildConfig.aiBaseURL?.host() ?? "live"))
            LabeledContent("appconfig", value: services.isLive ? (BuildConfig.appConfigBaseURL?.host() ?? "live") : "インメモリ")
            LabeledContent("バージョン", value: Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "-")
        }
        .font(.footnote)
    }

    private var statusText: String {
        switch services.transcriber.status {
        case .notDownloaded: "未ダウンロード"
        case .downloading: "ダウンロード中 \(Int(services.transcriber.downloadProgress * 100))%"
        case .loading: "読み込み中"
        case .ready: "準備完了"
        case .recognizing: "認識中"
        case .failed(let m): "失敗: \(m)"
        }
    }

    private var statusColor: Color {
        switch services.transcriber.status {
        case .ready, .recognizing: .green
        case .failed: .red
        default: .orange
        }
    }
}

#Preview {
    let services = BuildConfig.make(environment: .mock)
    SettingsSheet()
        .environment(AppRuntime(document: .fallback, services: services))
        .environment(AuthSession(service: services.auth))
        .environment(\.services, services)
}
