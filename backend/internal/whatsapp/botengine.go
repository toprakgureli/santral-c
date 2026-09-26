package whatsapp

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// A chatbot is a flow drawn in the panel: boxes (nodes) joined by arrows
// (edges). The engine below walks it for one customer. It does not talk to
// the database or to Meta itself; everything it does goes through botIO,
// so the same engine runs for real customers and in the test screen.

// BotGraph is a drawn flow.
type BotGraph struct {
	Nodes []BotNode `json:"nodes"`
	Edges []BotEdge `json:"edges"`
}

// BotNode is one box.
type BotNode struct {
	ID   string  `json:"id"`
	Type string  `json:"type"` // start | message | menu | ask | condition | api | tag | handoff | callback | survey | end
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Data BotData `json:"data"`
}

// BotData is a box's settings; each type uses its own fields.
type BotData struct {
	Text        string      `json:"text,omitempty"`
	MediaURL    string      `json:"mediaUrl,omitempty"`
	FileID      uint        `json:"fileId,omitempty"`    // a file uploaded from the panel
	FileName    string      `json:"fileName,omitempty"`  // its name, for the drawing
	MediaKind   string      `json:"mediaKind,omitempty"` // image | video | document
	Style       string      `json:"style,omitempty"`     // buttons | list
	ButtonLabel string      `json:"buttonLabel,omitempty"`
	Options     []BotOption `json:"options,omitempty"`
	Var         string      `json:"var,omitempty"`
	Validate    string      `json:"validate,omitempty"` // any | number | email | phone
	Retry       string      `json:"retry,omitempty"`
	Match       string      `json:"match,omitempty"` // all | any
	Rules       []BotRule   `json:"rules,omitempty"`
	Integration uint        `json:"integration,omitempty"`
	Map         []BotMap    `json:"map,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
	Priority    string      `json:"priority,omitempty"`
	Category    string      `json:"category,omitempty"`
	TeamID      uint        `json:"teamId,omitempty"`
	Note        string      `json:"note,omitempty"`
	Resolve     bool        `json:"resolve,omitempty"`
}

// BotOption is a menu choice; each has its own outgoing arrow.
type BotOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// BotRule is one test in a condition box.
type BotRule struct {
	Var   string `json:"var"`
	Op    string `json:"op"` // equals | not_equals | contains | gt | lt | exists | empty | hours_open | hours_closed
	Value string `json:"value"`
}

// BotMap copies a field of an outside system's answer into a variable.
type BotMap struct {
	Var  string `json:"var"`
	Path string `json:"path"` // e.g. data.status
}

// BotEdge is an arrow from a box's exit (port) to another box.
type BotEdge struct {
	ID   string `json:"id"`
	From string `json:"from"`
	Port string `json:"port"`
	To   string `json:"to"`
}

// botState is where a customer is in the flow.
type botState struct {
	NodeID string            `json:"nodeId"`
	Vars   map[string]string `json:"vars"`
	Tries  int               `json:"tries"`
	Done   bool              `json:"done"`
}

// botIO is everything the flow may do in the world.
type botIO interface {
	sendText(text string)
	sendMedia(kind, url string, fileID uint, fileName, caption string)
	sendMenu(style, text, button string, options []BotOption)
	handoff(teamID uint, note string)
	finish(resolve bool)
	tag(tags []string, priority, category string)
	callback(note string)
	survey()
	callAPI(integrationID uint, vars map[string]string) (map[string]any, error)
	hoursOpen() bool
	mark(nodeID, kind string)
}

func (g *BotGraph) node(id string) *BotNode {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			return &g.Nodes[i]
		}
	}
	return nil
}

func (g *BotGraph) start() *BotNode {
	for i := range g.Nodes {
		if g.Nodes[i].Type == "start" {
			return &g.Nodes[i]
		}
	}
	return nil
}

func (g *BotGraph) next(from, port string) *BotNode {
	for _, e := range g.Edges {
		if e.From == from && (e.Port == port || (port == "next" && e.Port == "")) {
			return g.node(e.To)
		}
	}
	return nil
}

// Validate checks a flow before it is published and says what is wrong
// in words.
func (g *BotGraph) Validate() []string {
	var problems []string
	starts := 0
	for _, n := range g.Nodes {
		if n.Type == "start" {
			starts++
		}
	}
	if starts != 1 {
		problems = append(problems, "Akışta tek bir Başlangıç kutusu olmalı.")
	}
	for _, n := range g.Nodes {
		label := boxName(n.Type)
		switch n.Type {
		case "message":
			if strings.TrimSpace(n.Data.Text) == "" && n.Data.MediaURL == "" && n.Data.FileID == 0 {
				problems = append(problems, label+" kutusunda metin ya da dosya yok.")
			}
		case "menu":
			if strings.TrimSpace(n.Data.Text) == "" {
				problems = append(problems, "Menü kutusunda soru metni yok.")
			}
			if len(n.Data.Options) == 0 {
				problems = append(problems, "Menü kutusunda seçenek yok.")
			}
			max := 10
			if n.Data.Style != "list" {
				max = 3
			}
			if len(n.Data.Options) > max {
				problems = append(problems, fmt.Sprintf("Menü düğmeli olduğunda en fazla %d seçenek alır; daha fazlası için liste türünü seçin.", max))
			}
			for _, o := range n.Data.Options {
				limit := 20
				if n.Data.Style == "list" {
					limit = 24
				}
				if utf8.RuneCountInString(o.Label) > limit || strings.TrimSpace(o.Label) == "" {
					problems = append(problems, fmt.Sprintf("Menü seçeneği boş olamaz ve en fazla %d karakter olabilir: %q", limit, o.Label))
				}
				if g.next(n.ID, o.ID) == nil {
					problems = append(problems, fmt.Sprintf("%q seçeneğinden bir kutuya ok çıkmıyor.", o.Label))
				}
			}
		case "ask":
			if strings.TrimSpace(n.Data.Text) == "" || strings.TrimSpace(n.Data.Var) == "" {
				problems = append(problems, "Soru kutusunda soru metni ve cevabın kaydedileceği değişken adı olmalı.")
			}
		case "api":
			if n.Data.Integration == 0 {
				problems = append(problems, "Dış sorgu kutusunda hangi sistemin sorgulanacağı seçilmemiş.")
			}
		}
		if n.Type != "handoff" && n.Type != "end" && n.Type != "menu" {
			hasOut := false
			for _, e := range g.Edges {
				if e.From == n.ID {
					hasOut = true
				}
			}
			if !hasOut && n.Type != "survey" && n.Type != "callback" {
				problems = append(problems, label+" kutusundan çıkan ok yok; akış orada biter.")
			}
		}
	}
	return problems
}

func boxName(t string) string {
	switch t {
	case "start":
		return "Başlangıç"
	case "message":
		return "Mesaj"
	case "menu":
		return "Menü"
	case "ask":
		return "Soru"
	case "condition":
		return "Koşul"
	case "api":
		return "Dış sorgu"
	case "tag":
		return "Etiket"
	case "handoff":
		return "Temsilciye aktar"
	case "callback":
		return "Geri arama"
	case "survey":
		return "Anket"
	case "end":
		return "Bitir"
	}
	return t
}

var varRef = regexp.MustCompile(`\{([a-zA-Z0-9_ğüşıöçĞÜŞİÖÇ]+)\}`)

func fillVars(text string, vars map[string]string) string {
	return varRef.ReplaceAllStringFunc(text, func(m string) string {
		k := strings.Trim(m, "{}")
		if v, ok := vars[k]; ok {
			return v
		}
		return m
	})
}

// botInput is what the customer answered.
type botInput struct {
	Text     string
	ChoiceID string // from a button or list reply
}

// step moves the flow forward with the customer's answer (nil at the
// start) until it needs another answer or ends.
func step(g *BotGraph, st *botState, in *botInput, io botIO) {
	if st.Vars == nil {
		st.Vars = map[string]string{}
	}
	var cur *BotNode
	if st.NodeID == "" {
		cur = g.start()
		if cur == nil {
			st.Done = true
			io.finish(false)
			return
		}
		io.mark(cur.ID, "enter")
		cur = g.next(cur.ID, "next")
	} else {
		at := g.node(st.NodeID)
		if at == nil {
			st.Done = true
			io.handoff(0, "Chatbot akışı değişti, müşteri temsilciye aktarıldı.")
			return
		}
		cur = answer(g, at, st, in, io)
		if cur == nil {
			return // waiting again, or ended inside answer
		}
	}
	for steps := 0; steps < 40; steps++ {
		if cur == nil {
			st.Done = true
			io.finish(false)
			return
		}
		io.mark(cur.ID, "enter")
		d := cur.Data
		switch cur.Type {
		case "start":
			cur = g.next(cur.ID, "next")
		case "message":
			if d.MediaURL != "" || d.FileID > 0 {
				io.sendMedia(d.MediaKind, d.MediaURL, d.FileID, d.FileName, fillVars(d.Text, st.Vars))
			} else if t := strings.TrimSpace(fillVars(d.Text, st.Vars)); t != "" {
				io.sendText(t)
			}
			cur = g.next(cur.ID, "next")
		case "menu":
			io.sendMenu(d.Style, fillVars(d.Text, st.Vars), d.ButtonLabel, d.Options)
			st.NodeID, st.Tries = cur.ID, 0
			return
		case "ask":
			io.sendText(fillVars(d.Text, st.Vars))
			st.NodeID, st.Tries = cur.ID, 0
			return
		case "condition":
			port := "no"
			if evalRules(d, st.Vars, io) {
				port = "yes"
			}
			cur = g.next(cur.ID, port)
		case "api":
			res, err := io.callAPI(d.Integration, st.Vars)
			if err != nil {
				cur = g.next(cur.ID, "fail")
				continue
			}
			for _, m := range d.Map {
				if v, ok := lookupPath(res, m.Path); ok {
					st.Vars[m.Var] = v
				}
			}
			cur = g.next(cur.ID, "ok")
		case "tag":
			io.tag(d.Tags, d.Priority, d.Category)
			cur = g.next(cur.ID, "next")
		case "callback":
			note := fillVars(d.Note, st.Vars)
			io.callback(note)
			if t := strings.TrimSpace(fillVars(d.Text, st.Vars)); t != "" {
				io.sendText(t)
			}
			cur = g.next(cur.ID, "next")
			if cur == nil {
				st.Done = true
				io.finish(d.Resolve)
				return
			}
		case "survey":
			io.survey()
			cur = g.next(cur.ID, "next")
			if cur == nil {
				st.Done = true
				io.finish(true)
				return
			}
		case "handoff":
			if t := strings.TrimSpace(fillVars(d.Text, st.Vars)); t != "" {
				io.sendText(t)
			}
			st.Done = true
			io.mark(cur.ID, "handoff")
			io.handoff(d.TeamID, fillVars(d.Note, st.Vars))
			return
		case "end":
			if t := strings.TrimSpace(fillVars(d.Text, st.Vars)); t != "" {
				io.sendText(t)
			}
			st.Done = true
			io.mark(cur.ID, "end")
			io.finish(d.Resolve)
			return
		default:
			cur = g.next(cur.ID, "next")
		}
	}
	// A loop in the drawing; hand over rather than spin.
	st.Done = true
	io.handoff(0, "Chatbot akışında döngü var, müşteri temsilciye aktarıldı.")
}

// answer handles the customer's reply at a box that asked for one and
// returns the box to continue with, or nil.
func answer(g *BotGraph, at *BotNode, st *botState, in *botInput, io botIO) *BotNode {
	if in == nil {
		return nil
	}
	d := at.Data
	retry := func(defaultText string) *BotNode {
		st.Tries++
		io.mark(at.ID, "fail")
		if st.Tries >= 2 {
			if n := g.next(at.ID, "fallback"); n != nil {
				return n
			}
			st.Done = true
			io.mark(at.ID, "handoff")
			io.handoff(0, "Chatbot müşterinin cevabını anlamadı.")
			return nil
		}
		text := strings.TrimSpace(d.Retry)
		if text == "" {
			text = defaultText
		}
		io.sendText(fillVars(text, st.Vars))
		if at.Type == "menu" {
			io.sendMenu(d.Style, fillVars(d.Text, st.Vars), d.ButtonLabel, d.Options)
		}
		return nil
	}
	switch at.Type {
	case "menu":
		choice := matchOption(d.Options, in)
		if choice == nil {
			return retry("Anlayamadım, lütfen seçeneklerden birini seçin.")
		}
		io.mark(at.ID, "answer")
		if d.Var != "" {
			st.Vars[d.Var] = choice.Label
		}
		st.Tries = 0
		return g.next(at.ID, choice.ID)
	case "ask":
		val := strings.TrimSpace(in.Text)
		if val == "" || !validAnswer(d.Validate, val) {
			return retry(validationHint(d.Validate))
		}
		io.mark(at.ID, "answer")
		st.Vars[d.Var] = val
		st.Tries = 0
		return g.next(at.ID, "next")
	}
	return g.next(at.ID, "next")
}

func matchOption(opts []BotOption, in *botInput) *BotOption {
	if in.ChoiceID != "" {
		id := strings.TrimPrefix(in.ChoiceID, "opt:")
		for i := range opts {
			if opts[i].ID == id {
				return &opts[i]
			}
		}
	}
	t := strings.ToLower(strings.TrimSpace(in.Text))
	if t == "" {
		return nil
	}
	if n, err := strconv.Atoi(strings.TrimRight(t, ".)")); err == nil && n >= 1 && n <= len(opts) {
		return &opts[n-1]
	}
	for i := range opts {
		if strings.ToLower(strings.TrimSpace(opts[i].Label)) == t {
			return &opts[i]
		}
	}
	return nil
}

var digitsOnlyRe = regexp.MustCompile(`^[0-9 .,]+$`)

func validAnswer(kind, v string) bool {
	switch kind {
	case "number":
		return digitsOnlyRe.MatchString(v)
	case "email":
		_, err := mail.ParseAddress(v)
		return err == nil
	case "phone":
		return len(digitsOnly(v)) >= 10 && len(digitsOnly(v)) <= 15
	}
	return true
}

func validationHint(kind string) string {
	switch kind {
	case "number":
		return "Lütfen yalnızca rakam yazın."
	case "email":
		return "Lütfen geçerli bir e-posta adresi yazın."
	case "phone":
		return "Lütfen telefon numaranızı başında 0 olmadan 10 haneli yazın."
	}
	return "Lütfen bir cevap yazın."
}

func evalRules(d BotData, vars map[string]string, io botIO) bool {
	if len(d.Rules) == 0 {
		return true
	}
	any := d.Match == "any"
	for _, r := range d.Rules {
		ok := evalRule(r, vars, io)
		if any && ok {
			return true
		}
		if !any && !ok {
			return false
		}
	}
	return !any
}

func evalRule(r BotRule, vars map[string]string, io botIO) bool {
	v := vars[r.Var]
	switch r.Op {
	case "hours_open":
		return io.hoursOpen()
	case "hours_closed":
		return !io.hoursOpen()
	case "exists":
		return strings.TrimSpace(v) != ""
	case "empty":
		return strings.TrimSpace(v) == ""
	case "equals":
		return strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(r.Value))
	case "not_equals":
		return !strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(r.Value))
	case "contains":
		return strings.Contains(strings.ToLower(v), strings.ToLower(r.Value))
	case "gt", "lt":
		a, err1 := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
		b, err2 := strconv.ParseFloat(strings.ReplaceAll(r.Value, ",", "."), 64)
		if err1 != nil || err2 != nil {
			return false
		}
		if r.Op == "gt" {
			return a > b
		}
		return a < b
	}
	return false
}

// lookupPath reads data.items.0.name out of a decoded JSON answer.
func lookupPath(v any, path string) (string, bool) {
	cur := v
	for _, part := range strings.Split(strings.TrimSpace(path), ".") {
		if part == "" {
			continue
		}
		switch x := cur.(type) {
		case map[string]any:
			cur = x[part]
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(x) {
				return "", false
			}
			cur = x[i]
		default:
			return "", false
		}
	}
	switch x := cur.(type) {
	case nil:
		return "", false
	case string:
		return x, true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	case bool:
		if x {
			return "evet", true
		}
		return "hayır", true
	default:
		b, _ := json.Marshal(x)
		return string(b), true
	}
}

// menuMessage builds WhatsApp's interactive message for a menu box.
func menuMessage(style, text, button string, options []BotOption) map[string]any {
	if style == "list" || len(options) > 3 {
		if strings.TrimSpace(button) == "" {
			button = "Seçenekler"
		}
		rows := make([]map[string]any, 0, len(options))
		for _, o := range options {
			row := map[string]any{"id": "opt:" + o.ID, "title": truncate(o.Label, 24)}
			if o.Description != "" {
				row["description"] = truncate(o.Description, 72)
			}
			rows = append(rows, row)
		}
		return map[string]any{"type": "interactive", "interactive": map[string]any{
			"type": "list", "body": map[string]any{"text": truncate(text, 1024)},
			"action": map[string]any{"button": truncate(button, 20), "sections": []map[string]any{{"title": "Seçenekler", "rows": rows}}},
		}}
	}
	btns := make([]map[string]any, 0, len(options))
	for _, o := range options {
		btns = append(btns, map[string]any{"type": "reply", "reply": map[string]any{"id": "opt:" + o.ID, "title": truncate(o.Label, 20)}})
	}
	return map[string]any{"type": "interactive", "interactive": map[string]any{
		"type": "button", "body": map[string]any{"text": truncate(text, 1024)}, "action": map[string]any{"buttons": btns},
	}}
}

func truncate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}

// simIO records what the flow would do, for the test screen.
type simIO struct {
	Out   []SimOutput
	hours bool
	api   func(uint, map[string]string) (map[string]any, error)
}

// SimOutput is one thing the flow did in the test screen.
type SimOutput struct {
	Kind    string      `json:"kind"` // text | media | menu | handoff | end | tag | callback | survey | api
	Text    string      `json:"text,omitempty"`
	Options []BotOption `json:"options,omitempty"`
	Style   string      `json:"style,omitempty"`
	Detail  string      `json:"detail,omitempty"`
}

func (o *simIO) sendText(t string) { o.Out = append(o.Out, SimOutput{Kind: "text", Text: t}) }
func (o *simIO) sendMedia(kind, url string, fileID uint, fileName, caption string) {
	what := url
	if fileID > 0 {
		what = fileName
	}
	o.Out = append(o.Out, SimOutput{Kind: "media", Text: caption, Detail: what})
}
func (o *simIO) sendMenu(style, text, button string, options []BotOption) {
	o.Out = append(o.Out, SimOutput{Kind: "menu", Text: text, Options: options, Style: style, Detail: button})
}
func (o *simIO) handoff(teamID uint, note string) {
	o.Out = append(o.Out, SimOutput{Kind: "handoff", Text: note, Detail: fmt.Sprint(teamID)})
}
func (o *simIO) finish(resolve bool) {
	d := ""
	if resolve {
		d = "resolve"
	}
	o.Out = append(o.Out, SimOutput{Kind: "end", Detail: d})
}
func (o *simIO) tag(tags []string, priority, category string) {
	o.Out = append(o.Out, SimOutput{Kind: "tag", Text: strings.Join(tags, ", "), Detail: strings.TrimSpace(priority + " " + category)})
}
func (o *simIO) callback(note string) { o.Out = append(o.Out, SimOutput{Kind: "callback", Text: note}) }
func (o *simIO) survey()              { o.Out = append(o.Out, SimOutput{Kind: "survey"}) }
func (o *simIO) callAPI(id uint, vars map[string]string) (map[string]any, error) {
	if o.api == nil {
		return nil, fmt.Errorf("sorgu yok")
	}
	res, err := o.api(id, vars)
	detail := "başarılı"
	if err != nil {
		detail = "başarısız: " + err.Error()
	}
	o.Out = append(o.Out, SimOutput{Kind: "api", Detail: detail})
	return res, err
}
func (o *simIO) hoursOpen() bool          { return o.hours }
func (o *simIO) mark(nodeID, kind string) {}

var _ = time.Now
