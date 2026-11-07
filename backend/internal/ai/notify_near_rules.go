package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// FeatureNotificationNear is a supportedFeatures value: the iOS build
// monitors Notification.near places and shifts times by jitterMinutes. Older
// builds read a near notification as a plain repeat once (a 09:00 reminder),
// so neither is generated for them (restrict.go calls the helpers below).
const FeatureNotificationNear = "notification.near"

// notifyNearRules: "目的地に近づいたら知らせて" is app.notifications[].near (iOS
// region monitoring on the device); "ランダムな時間に" is jitterMinutes on a
// time notification. Appended like alarmRules so prompt.go stays untouched.
var notifyNearRules = strings.TrimSpace(`
## 場所に着いたら知らせる通知は app.notifications の near
- 「目的地に近づいたら知らせて」「降りる駅の近くで教えて(乗り過ごし防止)」「お店の近くを通ったら買う物を知らせて」「危険な場所に近づいたら警告」: 場所の entity(location 型の field・入力は locationButton / locationPicker・地図は mapView)を作り、app.notifications に {id, title, body, repeat, **near: {entity, locationField, radiusM}**} を 1 件書く。entity の各レコードの場所が見張る場所になる。
- radiusM は 100〜2000 の m(省略 200。駅・乗り過ごしは 800〜1000、お店は 150〜300、危険な場所は 200〜300)。repeat once = 最初に着いた 1 回だけ(目的地・乗り過ごし)、daily = 着くたび(お店・危険な場所)。
- near を書いたら time・weekday・entity・field・offsetMinutes・titleField・enabledField・when・alarm・jitterMinutes は書かない(backend が拒否する)。scheduleNotification には書けない。
- 判定は**この端末の中**(位置はサーバーに送らない)。iOS が一度に見張れるのは近い順・新しい順に 15 か所まで。位置情報の許可が無い端末では届かない。summary は「保存した場所の近くに着いたら知らせます」程度にし、残りの距離を通知に出す・経路の途中で鳴らすことは約束しない(道案内は別の部品)。
- 編集ターン: 「近づいたら教えて」は add /app/notifications/-(無ければ add /app/notifications に配列)、「もっと手前で」は replace /app/notifications/<i>/near/radiusM。

## 通知の時刻を毎回ずらす(jitterMinutes)
- 「ランダムな時間に通知」「毎回違う時間に」「予測できない時間に」: 時刻の通知(time か entity + field)に jitterMinutes(1〜60)を足す。回ごとに ±N 分の別の時刻に鳴る。例: 19〜21 時のどこか = time "20:00" + jitterMinutes 60。
- 「期限を内緒で前倒し」「いつも少し早めに・何分前かは分からないように」: レコードの時刻の通知に offsetMinutes -N と jitterMinutes N(0〜2N 分前に鳴る)。
- when・near・scheduleNotification には付けない。「ランダムにはできない」と申告しない。
`)

// notifyNearChatRule: the same capability for chat answers (できる?).
var notifyNearChatRule = strings.TrimSpace(`
## 場所に着いたら知らせる・時刻をずらす通知
- 保存した場所の近く(100〜2000 m)に着いたら知らせる通知は**できる**(この端末の中で判定し、位置は送らない。位置情報の許可が要る・見張れるのは 15 か所まで)。残りの距離を知らせ続けるのはできない(道案内は別の部品でできる)。
- 通知の時刻を毎回ランダムに(±60 分まで)ずらすのも**できる**。
`)

// noNotifyNearChatRule replaces notifyNearChatRule for builds without FeatureNotificationNear.
const noNotifyNearChatRule = "## 場所に着いたら知らせる・時刻をずらす通知\n- このアプリの版では、場所に着いた時の通知と時刻をランダムにずらす通知はできない。頼まれたら「この版ではできません。最新版に更新すると使えます」と答え、地図・決まった時刻の通知で近い形を添える。"

// noNearNote is the restriction note line for builds without FeatureNotificationNear.
const noNearNote = "- 通知の near(場所に着いたら)と jitterMinutes(時刻をずらす)は使わない(この版では動かない)。頼まれたら地図や決まった時刻の通知で近い形を作り「場所に着いた時の通知・ランダムな時刻はこの版ではできないため、○○で代替しました。」と申告する。\n"

func init() {
	systemPrompt += "\n\n" + notifyNearRules
	with := chatCapabilities + "\n\n" + notifyNearChatRule
	chatEditSystemPrompt = strings.Replace(chatEditSystemPrompt, chatCapabilities, with, 1)
	chatCreateSystemPrompt = strings.Replace(chatCreateSystemPrompt, chatCapabilities, with, 1)
}

// restrictNearSchema drops near / jitterMinutes from the Notification def.
func restrictNearSchema(defs map[string]any) {
	if n, _ := defs["Notification"].(map[string]any); n != nil {
		props, _ := n["properties"].(map[string]any)
		delete(props, "near")
		delete(props, "jitterMinutes")
	}
}

// dropNearRules removes the near / jitter rules from a prompt.
func dropNearRules(s string) string {
	s = strings.Replace(s, "\n\n"+notifyNearRules, "", 1)
	return strings.Replace(s, notifyNearChatRule, noNotifyNearChatRule, 1)
}

// nearViolations lists near / jitter notifications in raw that base lacks.
func nearViolations(raw, base []byte) []string {
	prev := map[string]bool{}
	for _, id := range nearNotifications(base) {
		prev[id] = true
	}
	var out []string
	for _, id := range nearNotifications(raw) {
		if !prev[id] {
			out = append(out, fmt.Sprintf("app.notifications %q: near / jitterMinutes are not available in this app build: remove near (and the notification) or jitterMinutes, and substitute (e.g. a map or a fixed-time notification)", id))
		}
	}
	return out
}

func nearNotifications(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var app dsl.App
	if err := json.Unmarshal(raw, &app); err != nil {
		return nil
	}
	var out []string
	for _, n := range app.App.Notifications {
		if n.Near != nil || n.JitterMinutes != 0 {
			out = append(out, n.ID)
		}
	}
	return out
}

// unnear drops near notifications whole (an old build would read the rest
// as a 09:00 reminder) and the jitterMinutes key (an old build ignores it);
// it returns how many places changed.
func unnear(root map[string]any) int {
	app, _ := root["app"].(map[string]any)
	ns, ok := app["notifications"].([]any)
	if !ok {
		return 0
	}
	changed := 0
	kept := make([]any, 0, len(ns))
	for _, v := range ns {
		n, _ := v.(map[string]any)
		if n != nil && n["near"] != nil {
			changed++
			continue
		}
		if _, has := n["jitterMinutes"]; has {
			delete(n, "jitterMinutes")
			changed++
		}
		kept = append(kept, v)
	}
	app["notifications"] = kept
	return changed
}
