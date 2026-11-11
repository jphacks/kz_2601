import Foundation

/// 気づきにくい操作の吹き出し(#1441)。出す順番: 同意 → 最初の案内(#274)→ 最初のアプリ → 完成の後のお知らせ(`PostBuildNotice`)
/// → β の案内(#313)→ 吹き出し(1 日 1 つ)。どれを出すかの決まりは `CoachMarkSchedule`(Shared)。
extension AppRuntime {
    /// 吹き出しを出してよい時か。先の案内・お知らせ・シート・生成中の間は出さない。
    var canShowCoachMarks: Bool {
        guard hasSeenTutorial, hasMadeFirstApp, activeNotice == nil,
              !showTutorial, !showAIConsent, !showVoiceSheet,
              morphJob == nil, buildStage == nil else { return false }
        #if !APP_STORE_REVIEW
        if showBetaWelcome || isBetaWelcomePending { return false }
        #endif
        return true
    }

    /// この場面(`eligible` = いま使える操作)で出す吹き出し。
    func coachMark(eligible: Set<CoachMark>) -> CoachMark? {
        guard let coachMarks, canShowCoachMarks else { return nil }
        return coachMarks.next(eligible: eligible)
    }

    /// マイクのボタン(FAB)に出す吹き出し。変身一覧が上に出ている間は出さない(シートの下に隠れるため)。
    var fabCoachMark: CoachMark? {
        showAppSwitcher ? nil : coachMark(eligible: CoachMark.fabMarks)
    }

    /// 吹き出しで起きたこと。
    func handleCoachMark(_ mark: CoachMark, _ event: CoachMarkEvent) {
        guard let coachMarks else { return }
        switch event {
        case .shown: coachMarks.markShown(mark)
        case .dismissed: coachMarks.dismiss(mark)
        case .neverAgain:
            coachMarks.dismiss(mark)
            coachMarks.optOut()
        }
    }

    /// 実際にその操作をした(吹き出しを出す前でも)。次から出さない。
    func noteGesturePerformed(_ mark: CoachMark) {
        coachMarks?.markPerformed(mark)
    }

    /// 設定の「操作のヒントをもう一度見る」。シートを閉じきってから、最初の吹き出しが出る画面に戻る。
    func replayCoachMarks() {
        showVoiceSheet = false
        showAppSwitcher = false // 変身一覧の歯車から開いた設定(H10)
        coachMarks?.replay()
    }
}

enum CoachMarkEvent {
    case shown, dismissed, neverAgain
}
