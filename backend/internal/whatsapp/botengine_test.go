package whatsapp

import "testing"

func sampleFlow() *BotGraph {
	return &BotGraph{
		Nodes: []BotNode{
			{ID: "s", Type: "start"},
			{ID: "hi", Type: "message", Data: BotData{Text: "Merhaba {musteri}"}},
			{ID: "m", Type: "menu", Data: BotData{Text: "Konu?", Style: "buttons", Options: []BotOption{{ID: "sales", Label: "Satış"}, {ID: "support", Label: "Destek"}}}},
			{ID: "ask", Type: "ask", Data: BotData{Text: "Abone numaranız?", Var: "abone", Validate: "number"}},
			{ID: "c", Type: "condition", Data: BotData{Rules: []BotRule{{Var: "abone", Op: "gt", Value: "100"}}}},
			{ID: "h1", Type: "handoff", Data: BotData{TeamID: 7, Note: "Abone {abone}"}},
			{ID: "e", Type: "end", Data: BotData{Text: "Görüşmek üzere", Resolve: true}},
		},
		Edges: []BotEdge{
			{From: "s", Port: "next", To: "hi"},
			{From: "hi", Port: "next", To: "m"},
			{From: "m", Port: "sales", To: "e"},
			{From: "m", Port: "support", To: "ask"},
			{From: "ask", Port: "next", To: "c"},
			{From: "c", Port: "yes", To: "h1"},
			{From: "c", Port: "no", To: "e"},
		},
	}
}

func TestFlowWalksMenuAskConditionHandoff(t *testing.T) {
	g := sampleFlow()
	st := &botState{Vars: map[string]string{"musteri": "Ayşe"}}
	io := &simIO{}
	step(g, st, nil, io)
	if len(io.Out) != 2 || io.Out[0].Text != "Merhaba Ayşe" || io.Out[1].Kind != "menu" {
		t.Fatalf("start output wrong: %+v", io.Out)
	}
	if st.NodeID != "m" {
		t.Fatalf("should wait at the menu, at %q", st.NodeID)
	}
	io.Out = nil
	step(g, st, &botInput{Text: "2"}, io) // "2" picks Destek
	if st.NodeID != "ask" || len(io.Out) != 1 || io.Out[0].Text != "Abone numaranız?" {
		t.Fatalf("second option should ask: %+v at %q", io.Out, st.NodeID)
	}
	io.Out = nil
	step(g, st, &botInput{Text: "abc"}, io) // not a number: asked again
	if st.Done || st.Tries != 1 || io.Out[0].Text != "Lütfen yalnızca rakam yazın." {
		t.Fatalf("invalid answer should retry once: %+v", io.Out)
	}
	io.Out = nil
	step(g, st, &botInput{Text: "250"}, io)
	if !st.Done || len(io.Out) != 1 || io.Out[0].Kind != "handoff" || io.Out[0].Text != "Abone 250" || io.Out[0].Detail != "7" {
		t.Fatalf("should hand off to team 7 with the variable filled: %+v", io.Out)
	}
}

func TestFlowButtonReplyAndEnd(t *testing.T) {
	g := sampleFlow()
	st := &botState{}
	io := &simIO{}
	step(g, st, nil, io)
	io.Out = nil
	step(g, st, &botInput{ChoiceID: "opt:sales"}, io)
	if !st.Done || io.Out[0].Text != "Görüşmek üzere" || io.Out[1].Kind != "end" || io.Out[1].Detail != "resolve" {
		t.Fatalf("sales should end and resolve: %+v", io.Out)
	}
}

func TestFlowHandsOverAfterTwoMisunderstandings(t *testing.T) {
	g := sampleFlow()
	st := &botState{}
	io := &simIO{}
	step(g, st, nil, io)
	step(g, st, &botInput{Text: "bilmem"}, io)
	io.Out = nil
	step(g, st, &botInput{Text: "yine bilmem"}, io)
	if !st.Done || io.Out[len(io.Out)-1].Kind != "handoff" {
		t.Fatalf("two misses should hand over: %+v", io.Out)
	}
}

func TestFlowLoopIsCut(t *testing.T) {
	g := &BotGraph{
		Nodes: []BotNode{{ID: "s", Type: "start"}, {ID: "a", Type: "tag"}, {ID: "b", Type: "tag"}},
		Edges: []BotEdge{{From: "s", To: "a"}, {From: "a", To: "b"}, {From: "b", To: "a"}},
	}
	st := &botState{}
	io := &simIO{}
	step(g, st, nil, io)
	if !st.Done || io.Out[len(io.Out)-1].Kind != "handoff" {
		t.Fatalf("a loop must end in a handoff: %+v", io.Out)
	}
}

func TestValidateNamesProblems(t *testing.T) {
	g := &BotGraph{Nodes: []BotNode{{ID: "m", Type: "menu", Data: BotData{Text: "x", Options: []BotOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}, {ID: "c", Label: "C"}, {ID: "d", Label: "D"}}}}}}
	p := g.Validate()
	if len(p) < 3 {
		t.Fatalf("missing start, too many buttons and missing arrows should all show: %v", p)
	}
	if ok := sampleFlow().Validate(); len(ok) != 0 {
		t.Fatalf("the sample flow is valid, got %v", ok)
	}
}

func TestLookupPath(t *testing.T) {
	v := map[string]any{"data": map[string]any{"items": []any{map[string]any{"status": "Kargoda", "n": 3.0}}}}
	if s, ok := lookupPath(v, "data.items.0.status"); !ok || s != "Kargoda" {
		t.Fatalf("got %q", s)
	}
	if s, _ := lookupPath(v, "data.items.0.n"); s != "3" {
		t.Fatalf("got %q", s)
	}
	if _, ok := lookupPath(v, "data.items.5"); ok {
		t.Fatal("out of range must fail")
	}
}
