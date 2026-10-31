package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Logic pass (#100): after the UI is generated (pass 1), a second, small call
// adds only the calculations the utterance needs (Field.computed / stat.value /
// settlement / ref fields) as a JSON Patch. It is opt-in (Service.LogicPass or
// Request.LogicPass) and best-effort: if the patch never validates, the pass-1
// app is kept and the problems are reported in Meta.LogicPass.

// LogicMeta reports what the logic pass did.
type LogicMeta struct {
	Applied  bool     `json:"applied"`  // a non-empty patch validated and was merged
	Ops      int      `json:"ops"`      // patch ops merged
	Attempts int      `json:"attempts"` // Claude calls of this pass
	ClaudeMs int64    `json:"claudeMs"`
	Usage    Usage    `json:"usage"`
	Problems []string `json:"problems,omitempty"` // last validation problems when not applied
}

// logicPassMaxAttempts bounds the pass (1 retry with the problems).
const logicPassMaxAttempts = 2

func (s *Service) logicEnabled(req Request) bool {
	if req.LogicPass != nil {
		return *req.LogicPass
	}
	return s.LogicPass
}

// exprCatalog is the calculation part of the catalog (from "### 計算式 Expr").
var exprCatalog = func() string {
	if i := strings.Index(catalog, "### 計算式 Expr"); i >= 0 {
		return catalog[i:]
	}
	return catalog
}()

// settlementLine is the settlement leaf's catalog line (pass 2 may add it).
var settlementLine = func() string {
	for _, l := range strings.Split(catalog, "\n") {
		if strings.HasPrefix(l, "- settlement:") {
			return l
		}
	}
	return ""
}()

// logicSystemPrompt is deliberately small (no UI catalog / schema) so the
// pass stays fast; it is static and prompt-cacheable.
var logicSystemPrompt = strings.TrimSpace(`
あなたは iOS アプリ(AppDSL v1)の「計算ロジック」担当です。UI は前の工程で作られています。ユーザーの発話に必要な**計算だけ**を、現在の DSL に対する JSON Patch(RFC 6902)で足します。
出力は {"summary":"...","patch":[...]} の JSON オブジェクト 1 つだけ(1 行の compact JSON・前置きなし)。

## 規則
- 足してよいもの: entity の field(computed を持つ算出列・type "ref"+refEntity の参照項目)、stat の value(Expr)、stat / settlement / table の部品、それらを表示するための list.item・table.columns の差し替え。
- 計算が要らない発話(記録・一覧だけ)なら patch は [] にする(空でよい)。既に計算が足りていても [] 。
- 既存の field id はリネームしない・UI の配置(area)は重ならないように。新しい部品は空いている行に置く(例: 最後に使われている行の次)か、既存部品の area を詰める。
- computed: type は number|integer|currency|string|boolean|date。ref は同じレコードのフィールド("id" はレコード id)。stat.value は画面の値なので ref 不可・agg を使う。stat.value を使うときは aggregate/entity/field/filter を書かない(既存 stat を置き換えるなら replace で丸ごと)。
- 他 entity のレコードを指す項目(払った人→メンバー)は type "ref"+refEntity。フォームではその field を picker で選ぶ(picker を add する)。
- 割り勘・立替の「誰が誰にいくら」は settlement。
- summary は足した計算を 1 文(ja なら日本語「1人あたりと精算額を計算するようにしました。」)。patch が [] なら「計算は不要でした。」。
- 配列 index は「現在の DSL」のもの。末尾追加は "/-"。

## 部品(計算で使うもの)
` + settlementLine + `
- stat: label / aggregate(count|sum|avg|min|max), entity, field, filter, format(plain|currency|percent), value(Expr)
- table: entity, columns[{field,title?}] / sort, filter, limit

` + exprCatalog)

// logicUserMessage is the pass-2 user turn.
func logicUserMessage(transcript string, current []byte, problems []string, loc Locale) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## 現在の DSL\n```json\n%s\n```\n\n", current)
	writeLocale(&b, loc)
	fmt.Fprintf(&b, "## ユーザーの発話\n「%s」\n\n", strings.TrimSpace(transcript))
	writeProblems(&b, problems)
	b.WriteString("## 出力\n{summary, patch: 計算を足す RFC 6902 操作の配列(不要なら [])}(1 行の compact JSON)")
	return b.String()
}

// runLogicPass returns the DSL with calculations merged (or the input when
// nothing applied) and the pass metadata. It never fails the request.
func (s *Service) runLogicPass(ctx context.Context, transcript string, current []byte, loc Locale) ([]byte, *LogicMeta) {
	lm := &LogicMeta{}
	var problems []string
	for i := 0; i < logicPassMaxAttempts; i++ {
		ar := &attemptResult{}
		resp, err := s.call(WithOperation(ctx, OpLogic), logicSystemPrompt, logicUserMessage(transcript, current, problems, loc), PatchGenerationSchema(), ar, nil)
		lm.Attempts++
		lm.ClaudeMs += ar.latency.Milliseconds()
		lm.Usage.Add(ar.meta.Usage)
		if err != nil {
			lm.Problems = []string{"logic pass call failed: " + err.Error()}
			return current, lm
		}
		var doc dsl.PatchDocument
		if uerr := json.Unmarshal(extractJSON(resp.Text), &doc); uerr != nil {
			problems = []string{"output is not a valid PatchDocument: " + uerr.Error()}
			lm.Problems = problems
			continue
		}
		if len(doc.Patch) == 0 {
			lm.Problems = nil
			return current, lm
		}
		ops := fillPatchDefaults(doc.Patch, current)
		next, _, perr := dsl.ApplyPatch(current, ops)
		if perr == nil {
			lm.Applied = true
			lm.Ops = len(ops)
			lm.Problems = nil
			return next, lm
		}
		problems = problemsOf(perr, resp)
		lm.Problems = problems
	}
	return current, lm
}

// applyLogicPass runs the pass on a validated full DSL and folds the result
// into ar (raw / summary / usage / latency / meta). Create turns only: the
// edit path already returns a patch, and a second patch would have to be
// merged with it (後続対応).
func (s *Service) applyLogicPass(ctx context.Context, transcript string, ar *attemptResult, loc Locale) {
	t := time.Now()
	next, lm := s.runLogicPass(ctx, transcript, ar.raw, loc)
	ar.latency += time.Since(t)
	ar.meta.Usage.Add(lm.Usage)
	ar.meta.LogicPass = lm
	if !lm.Applied {
		return
	}
	app, err := dsl.Parse(next)
	if err != nil { // ApplyPatch already parsed it; defensive
		return
	}
	ar.raw = canon(next)
	ar.summary = summarizeFor(app, loc)
}
