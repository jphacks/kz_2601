package ai

import (
	"context"
	"strings"
	"testing"
)

const taxPatch = `{"summary":"税額を計算するようにしました。","patch":[{"op":"add","path":"/entities/0/fields/-","value":{"id":"tax","name":"税","type":"currency","computed":{"op":"round","args":[{"op":"mul","args":[{"ref":"amount"},0.1]}]}}}]}`

func TestLogicPass_MergesCalculations(t *testing.T) {
	kb := kakeibo(t)
	f := &fakeCaller{replies: []string{string(kb), taxPatch}}
	svc, _ := newSvc(f)
	on := true
	res, err := svc.Generate(context.Background(), Request{AppID: "l1", Transcript: "家計簿で税額も見たい", LogicPass: &on})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.DSL), `"computed"`) {
		t.Fatalf("computed field not merged: %s", res.DSL)
	}
	lm := res.Meta.LogicPass
	if lm == nil || !lm.Applied || lm.Ops != 1 || lm.Attempts != 1 {
		t.Fatalf("logic meta: %+v", lm)
	}
	if res.Meta.Attempts != 1 || res.Meta.Usage.OutputTokens != 10 {
		t.Fatalf("pass-1 attempts must not include the logic pass; usage must: %+v", res.Meta)
	}
	if len(f.calls) != 2 || !strings.Contains(f.calls[0].Messages[0].Content, "UI の工程") {
		t.Fatal("pass 1 should be told calculations come later")
	}
	if f.calls[1].System != logicSystemPrompt || !strings.Contains(f.calls[1].Messages[0].Content, "家計簿で税額も見たい") {
		t.Fatal("pass 2 should use the logic prompt with the utterance")
	}
}

func TestLogicPass_KeepsUIWhenPatchNeverValidates(t *testing.T) {
	kb := kakeibo(t)
	bad := `{"summary":"x","patch":[{"op":"add","path":"/entities/0/fields/-","value":{"id":"tax","name":"税","type":"currency","computed":{"op":"median","args":[]}}}]}`
	f := &fakeCaller{replies: []string{string(kb), bad, bad}}
	svc, _ := newSvc(f)
	svc.LogicPass = true
	res, err := svc.Generate(context.Background(), Request{AppID: "l2", Transcript: "家計簿"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(res.DSL), `"computed"`) {
		t.Fatal("invalid logic patch must not be merged")
	}
	lm := res.Meta.LogicPass
	if lm == nil || lm.Applied || lm.Attempts != 2 || len(lm.Problems) == 0 || !strings.Contains(strings.Join(lm.Problems, ";"), "computed/op") {
		t.Fatalf("logic meta: %+v", lm)
	}
	if !strings.Contains(f.calls[2].Messages[0].Content, "computed/op") {
		t.Fatal("retry must carry the problems")
	}
}

func TestLogicPass_EmptyPatchAndOptOut(t *testing.T) {
	kb := kakeibo(t)
	f := &fakeCaller{replies: []string{string(kb), `{"summary":"計算は不要でした。","patch":[]}`}}
	svc, _ := newSvc(f)
	svc.LogicPass = true
	res, err := svc.Generate(context.Background(), Request{AppID: "l3", Transcript: "家計簿"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta.LogicPass == nil || res.Meta.LogicPass.Applied {
		t.Fatalf("empty patch: %+v", res.Meta.LogicPass)
	}
	off := false
	f2 := &fakeCaller{replies: []string{string(kb)}}
	svc2, _ := newSvc(f2)
	svc2.LogicPass = true
	res2, err := svc2.Generate(context.Background(), Request{AppID: "l4", Transcript: "家計簿", LogicPass: &off})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Meta.LogicPass != nil || len(f2.calls) != 1 {
		t.Fatal("request opt-out must skip the logic pass")
	}
}
