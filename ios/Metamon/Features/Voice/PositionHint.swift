import SwiftUI

/// summary(「右下のボタンから◯◯を追加できます」など)に含まれる位置語を画面上のアンカーに変換する。
/// 部品単位の特定は DSL と summary の対応が無いので行わず、領域(四隅・上下・左右)のハイライトに留める。
enum PositionHint {
    private static let table: [(String, UnitPoint)] = [
        ("右下", UnitPoint(x: 0.85, y: 0.85)),
        ("左下", UnitPoint(x: 0.15, y: 0.85)),
        ("右上", UnitPoint(x: 0.85, y: 0.15)),
        ("左上", UnitPoint(x: 0.15, y: 0.15)),
        ("一番下", UnitPoint(x: 0.5, y: 0.88)),
        ("一番上", UnitPoint(x: 0.5, y: 0.12)),
        ("下部", UnitPoint(x: 0.5, y: 0.85)),
        ("上部", UnitPoint(x: 0.5, y: 0.15)),
        ("中央", UnitPoint(x: 0.5, y: 0.5)),
        ("右", UnitPoint(x: 0.85, y: 0.5)),
        ("左", UnitPoint(x: 0.15, y: 0.5)),
        // en の summary(locale=en・#45)。小文字化して照合する
        ("bottom right", UnitPoint(x: 0.85, y: 0.85)),
        ("bottom left", UnitPoint(x: 0.15, y: 0.85)),
        ("top right", UnitPoint(x: 0.85, y: 0.15)),
        ("top left", UnitPoint(x: 0.15, y: 0.15)),
        ("at the bottom", UnitPoint(x: 0.5, y: 0.85)),
        ("at the top", UnitPoint(x: 0.5, y: 0.15)),
        ("in the center", UnitPoint(x: 0.5, y: 0.5)),
    ]

    /// 最初に出現する位置語を採用(「右下」は「右」より先に判定)。
    static func anchor(in summary: String) -> UnitPoint? {
        table.first { summary.lowercased().contains($0.0) }?.1
    }
}
