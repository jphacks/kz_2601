// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonDSL
import MetamonServices
import MetamonRenderer

/// 近距離共有(#36)の UI 状態。AppRuntime(@Observable)には拡張で保存プロパティを足せないので別に持つ(SharingUI と同じ形)。
@MainActor
@Observable
final class NearbyUI {
    static let shared = NearbyUI()

    struct SendRequest: Identifiable {
        let id = UUID()
        let payload: NearbyShare.Payload
    }

    /// 送信シート(相手選び)。nil でなければ `NearbySheets` が present する
    var request: SendRequest?
    /// 通信本体(`AppRuntime.startNearby()` で作る)
    private(set) var service: NearbyService?

    func install(_ service: NearbyService) { self.service = service }
}

extension AppRuntime: NearbyHost {
    /// `nearbyShare` leaf から: 送るデータを作って相手選びのシートを開く。
    func shareNearby(entity: String, records: [[String: JSONValue]]) {
        guard let e = dsl.entity(entity) else { return }
        let ui = NearbyUI.shared
        let sender = ui.service?.myName ?? nearbyPublicName
        let payload = NearbyShare.makePayload(sender: sender, appId: document.appId, appName: dsl.app.name, entity: e, records: records)
        guard payload.header.count > 0 else {
            toast = L10n.tr("送る記録がありません")
            return
        }
        ui.request = .init(payload: payload)
    }

    /// `musicPlayer` から(#412): 曲のリクエストを作って相手選びのシートを開く。
    func requestSong(_ song: MusicSong) {
        let sender = NearbyUI.shared.service?.myName ?? nearbyPublicName
        guard let payload = NearbyShare.makeSongRequest(song, sender: sender, appId: document.appId, appName: dsl.app.name) else {
            toast = L10n.tr("曲名を入れてください")
            return
        }
        NearbyUI.shared.request = .init(payload: payload)
    }

    /// 自分の表示名(相手の一覧と「◯◯さんから」に出る)。サインイン名 → 端末名。
    var nearbyDisplayName: String {
        if let name = services.auth.currentUser?.displayName, !name.isEmpty { return name }
        return UIDevice.current.name
    }

    /// 相手に見える名前(近距離共有の MCPeerID と Header の送り主)。すれ違い通信と同じ「名前を出さない」(`passby.anonymous`)に従う。
    var nearbyPublicName: String {
        NearbyLimits.peerName(nearbyDisplayName, anonymous: NearbyPassbyHost.controller.isAnonymous)
    }

    /// 「名前を出さない」を切り替えた後に呼ぶ: 近距離共有の名前を今の設定に揃える(変わらなければ何もしない)。
    func refreshNearbyName() {
        NearbyUI.shared.service?.rename(nearbyPublicName)
    }

    /// 起動時に 1 回。受信の判定と取り込みを結ぶ(advertise の開始は `updateNearbyAdvertising`)。
    func startNearby() {
        guard NearbyUI.shared.service == nil else { return }
        let service = NearbyService(displayName: nearbyPublicName)
        service.decide = { [weak self] header in
            guard let self else { return .reject(reason: L10n.tr("今は受け取れません")) }
            return NearbyShare.decide(header, dsl: self.dsl, appId: self.document.appId)
        }
        service.onReceive = { [weak self] payload, _ in
            guard let self else { return .init(ok: false, added: 0, message: L10n.tr("今は受け取れません")) }
            return await self.importNearby(payload)
        }
        NearbyUI.shared.install(service)
    }

    /// 前面にいて、今のアプリが `nearbyShare` を使っている間だけ待ち受ける。
    /// (使わないアプリでは advertise しない = 初回起動時にローカルネットワーク許可ダイアログを出さない)
    func updateNearbyAdvertising(foreground: Bool) {
        NearbyUI.shared.service?.setAdvertising(foreground && dsl.usesNearbyReceive)
    }

    /// 受け取ったレコードを今のアプリに追加する。招待後にアプリが切り替わっていることもあるので、ここでも判定し直す。
    func importNearby(_ payload: NearbyShare.Payload) async -> NearbyShare.Ack {
        let h = payload.header
        if let song = h.song { return await importSongRequest(song, from: NearbyLimits.displayName(h.sender)) }
        switch NearbyShare.decide(h, dsl: dsl, appId: document.appId) {
        case .reject(let reason):
            toast = reason
            return .init(ok: false, added: 0, message: reason)
        case .accept(let entityId, let sameApp):
            guard let entity = dsl.entity(entityId) else { return .init(ok: false, added: 0) }
            let rows = NearbyShare.sanitize(payload.records, into: entity, sameApp: sameApp)
            guard !rows.isEmpty else {
                toast = L10n.tr("\(NearbyLimits.displayName(h.sender))さんから届いた記録に、追加できる項目がありませんでした")
                return .init(ok: false, added: 0, message: L10n.tr("追加できる項目がありませんでした"))
            }
            let appId = document.appId
            for row in rows {
                _ = await recordSync.save(appId: appId, entity: entityId, recordId: nil, data: row, author: postAuthor(for: entityId))
            }
            await reloadLocal(entityId, appId: appId)
            toast = L10n.tr("\(NearbyLimits.displayName(h.sender))さんから「\(entity.name)」を \(rows.count) 件追加しました")
            return .init(ok: true, added: rows.count)
        }
    }
}

extension AppRuntime {
    /// 曲のリクエスト(#412)を musicPlayer の承認待ちへ(autoAccept なら再生待ちへ)。招待後に切り替わったアプリでは断る。
    func importSongRequest(_ song: MusicSong, from sender: String) async -> NearbyShare.Ack {
        guard let player = dsl.musicPlayer, player.allowRequests == true else {
            return .init(ok: false, added: 0, message: L10n.tr("相手のアプリは今、曲のリクエストを受け付けていません"))
        }
        return await MusicSession.shared.receive(song, from: sender, autoAccept: player.autoAccept == true)
    }
}
#endif
