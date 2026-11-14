import SwiftUI
import MetamonDSL
import MetamonRenderer

/// create の結果が届いた後の組み上げ演出(#626)。結果は適用済みで、演出の間だけ RootView が生成中の表示を続ける。
struct BuildReveal: Equatable {
    let id = UUID()
    /// 待っていた時の生成中(見出しの依頼文・経過秒をそのまま続ける)
    var job: AppRuntime.MorphJob
    var plan: BuildChoreography.AssemblyPlan
    /// 演出の間は RootView の `.id` をこの値(待っていた時の id)に留め、生成中の表示を作り直さない。
    /// 演出が終わって本物の id に変わる時に、いつもの変身のクロスフェードになる
    var heldViewID: String
}

extension AppRuntime {
    /// RootView の `.id`。組み上げの間は待っていた時の id に留める
    var rootViewID: String { buildReveal?.heldViewID ?? "\(document.appId)#\(document.version)" }

    /// 全画面の生成中の表示に出すもの(待っている間 = reveal なし / 組み上げ中 = reveal あり)。nil なら出さない
    var buildStage: (job: MorphJob, reveal: BuildReveal?)? {
        if let job = morphJob, job.kind == .create { return (job, nil) }
        if let reveal = buildReveal { return (reveal.job, reveal) }
        return nil
    }

    /// 結果を当てる時の見せ方。create → 組み上げ、edit → 変わった部品を光らせる。
    func buildStagePlan(job: MorphJob?, previous: AppDocument, next: AppDocument) -> (reveal: BuildReveal?, highlight: ChangeHighlight) {
        guard let job else { return (nil, .none) }
        switch job.kind {
        case .create:
            // 仮置き(#636)で見せた部品のうち結果と同じものは置いたまま、残りを置く
            let plan = BuildChoreography.AssemblyPlan.make(for: next.dsl, reduceMotion: reduceMotion(), shown: job.draft.parts)
            return (BuildReveal(job: job, plan: plan, heldViewID: rootViewID), .none)
        case .edit:
            guard previous.appId == next.appId, !previous.dsl.isVoiceOnly else { return (nil, .none) }
            return (nil, BuildChoreography.editHighlight(old: previous.dsl, new: next.dsl))
        }
    }

    /// 組み上げが終わったら本物の画面へ。光らせる印は出現が済んだ頃に消す(後でタブを開いた時に光らせ直さない)。
    func scheduleBuildRevealEnd(_ reveal: BuildReveal?, highlight: ChangeHighlight) {
        guard reveal != nil || !highlight.isEmpty else { return }
        buildRevealTask = Task { @MainActor [weak self] in
            if let reveal {
                try? await Task.sleep(for: .seconds(reveal.plan.total))
                guard !Task.isCancelled else { return }
                self?.finishBuildReveal(reveal.id)
            }
            if !highlight.isEmpty {
                try? await Task.sleep(for: .seconds(2))
                guard !Task.isCancelled, self?.changeHighlight == highlight else { return }
                self?.changeHighlight = .none
            }
        }
    }

    /// 組み上げを終える(時間切れ・タップで飛ばす)。別の apply が来た後なら何もしない。
    func finishBuildReveal(_ id: UUID? = nil) {
        guard let reveal = buildReveal, id == nil || reveal.id == id else { return }
        if id == nil { buildRevealTask?.cancel() }
        withAnimation(.spring(duration: 0.65, bounce: 0.12)) { buildReveal = nil }
    }
}
