import Foundation
import MetamonDSL
import MetamonServices

/// 他の端末(iPhone など)で変わった記録を、開いている画面に出す(#2072・iOS の `AppRuntime.startSync` の Mac 版)。
/// 購読は `MacRealtimeSync` が appId ごとに 1 本持ち、ここは借りて受け取るだけ。版の差し替えは一覧(`MacAppLibrary`)が行う。
extension MacAppRuntime {
    /// 購読を借りる。ランタイムを捨てる時(`deinit`)に返す
    func attachRealtime(_ lease: MacRealtimeLease?) {
        realtimeLease?.release()
        realtimeLease = lease
        guard let lease else { return }
        lease.onRecordsChanged = { [weak self] entity in self?.realtimeRecordsChanged(entity) }
        lease.onCatchUp = { [weak self] in self?.realtimeCatchUp() }
        lease.isInteracting = { [weak self] in self?.isInteractingForHotReload ?? false }
    }

    /// 入力・操作の途中。DSL を差し替えるとランタイムごと作り直して下書き・開いた画面が消えるので待つ(iOS の `isInteracting` と同じ考え方)
    var isInteractingForHotReload: Bool {
        sheet != nil || fullScreen != nil || paths.values.contains { !$0.isEmpty }
            || isShowingDialog || hasUnsavedInput() || gridEdit != nil // 並びの編集中(#2323)も待つ
    }

    /// 他の端末でこの entity が変わった。読み込んだ(表示中の)entity だけ取り直す。未表示の entity は画面が開いた時に読む
    func realtimeRecordsChanged(_ entity: String) {
        guard records[entity] != nil else { return }
        Task { await refresh(entity) }
    }

    /// 購読が切れていた・スリープしていた。表示中の entity を全部取り直す
    func realtimeCatchUp() {
        for entity in records.keys { realtimeRecordsChanged(entity) }
    }
}
