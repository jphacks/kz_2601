# サンプル（プロダクト名）

[![IMAGE ALT TEXT HERE](https://jphacks.com/wp-content/uploads/2026/07/IMG_5316.png)](https://www.youtube.com/watch?v=piaNsc6ilBI)

## 製品概要
### 背景(製品開発のきっかけ、課題等）
### 製品説明（具体的な製品の説明）
### 特長
#### 1. 特長1
#### 2. 特長2
#### 3. 特長3

### 解決出来ること
### 今後の展望
### 注力したこと（こだわり等）
* 
* 

## 開発技術
### 活用した技術
#### API・データ
* 
* 

#### フレームワーク・ライブラリ・モジュール
* 
* 

#### デバイス
* 
* 

### 独自技術
#### ハッカソンで開発した独自機能・技術
* 独自で開発したものの内容をこちらに記載してください
* 特に力を入れた部分をファイルリンク、またはcommit_idを記載してください。

## リポジトリ構成
iOS アプリ(`ios/`)とバックエンド(`backend/`)を同居させたモノレポ。詳細は [docs/00_decisions.md](docs/00_decisions.md)。

| パス | 内容 |
|---|---|
| `ios/` | iOS アプリ本体(Swift / SwiftUI、xcodegen) |
| `backend/` | Go サービス群(`cmd/appconfig`, `cmd/ai`)、Cloud Run |
| `docs/` | 設計・運用ドキュメント(正典は `00_decisions.md`) |
| `.github/` | CI・PR/Issue テンプレート |
| `scripts/` | 運用スクリプト |
