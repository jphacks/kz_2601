import Foundation
import MetamonServices

/// アプリごとのチャット履歴(#210)。保存・文脈は `ChatHistory.swift`。
/// - `chatHistory` = 閉じた会話(反映・切替で閉じたもの)と反映の区切り。今の会話(`chatBubbles`)は含まない
/// - 端末には `chatHistory + 今の会話` を保存する(吹き出しが増えるたび)。読み直すと全部が閉じた会話になる
/// - 「最初から」で全部消す。アプリ削除・アカウント削除でも消す
extension AppRuntime {
    /// 閉じた会話があるか。生成後のシートは、これがあれば今の会話が無くても吹き出しの画面を出す
    var hasChatHistory: Bool { !chatHistory.isEmpty }

    /// 今のアプリ(`document.appId`)の履歴を読み直す(起動・アプリの切替)。
    func reloadChatHistory() {
        chatHistory = chatHistoryStore.load(appId: document.appId)
    }

    /// 今の会話を閉じた会話に移す(`resetChat` の先頭で呼ぶ)。
    func archiveLiveChat() {
        let live = liveChatEntries
        guard !live.isEmpty else { return }
        chatHistory = ChatHistoryStore.trimmed(chatHistory + live)
        persistChatHistory()
    }

    /// 反映した所に区切りを入れる(generate の成功時)。要約は generate の summary。
    /// 対話を使っていないアプリ(履歴が空)には入れない
    func noteChatApplied(version: Int, summary: String?) {
        archiveLiveChat()
        guard hasChatHistory else { return }
        chatHistory = ChatHistoryStore.trimmed(chatHistory + [ChatHistoryEntry(kind: .applied, text: summary ?? "", version: version)])
        persistChatHistory()
    }

    /// 「最初から」: 今の会話も、このアプリの履歴も消す。
    func restartChat() {
        resetChat()
        chatHistory = []
        chatHistoryStore.remove(appId: document.appId)
    }

    /// 端末に保存する(閉じた会話 + 今の会話)。
    func persistChatHistory() {
        chatHistoryStore.save(chatHistory + liveChatEntries, appId: document.appId)
    }

    /// AI に送る messages(`ChatContext`)。
    var chatContextMessages: [ChatMessage] {
        ChatContext.messages(history: chatHistory,
                             live: chatBubbles.map { ChatMessage(role: $0.role == .user ? .user : .assistant, text: $0.text) })
    }

    private var liveChatEntries: [ChatHistoryEntry] {
        chatBubbles.map { ChatHistoryEntry(kind: $0.role == .user ? .user : .assistant, text: $0.text) }
    }
}
