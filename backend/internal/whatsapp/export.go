package whatsapp

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

var (
	unsafeName = regexp.MustCompile(`[^\p{L}\p{N}_-]+`)
	unsafeFile = regexp.MustCompile(`[^\p{L}\p{N}._-]+`)
)

// exportMediaLimit caps how much media one archive carries; past it the
// page says which files were left out.
const exportMediaLimit = 2 << 30

// Export is a conversation ready to be written as an archive: a page that
// reads like the chat, with every photo, video, voice note and document
// beside it in a folder.
type Export struct {
	s        *Service
	ch       *models.WAChannel
	contact  *models.WAContact
	msgs     []models.WAMessage
	names    map[uint]string
	by       string
	FileName string
}

// PrepareExport checks the person may export the conversation and loads it.
// Writing comes later, while the archive streams to the browser.
func (s *Service) PrepareExport(ctx context.Context, actorID, conversationID uint) (*Export, error) {
	v, conv, _, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return nil, err
	}
	if !v.can(enums.WAExport) {
		return nil, errs.Forbidden("Yazışmayı dışa aktarma yetkiniz yok.")
	}
	contact, err := s.contact(ctx, conv.ContactID)
	if err != nil {
		return nil, err
	}
	ch, err := s.channel(ctx, conv.ChannelID)
	if err != nil {
		return nil, err
	}
	var msgs []models.WAMessage
	if err := s.db.WithContext(ctx).Where("conversation_id = ? AND kind <> 'reaction'", conv.ID).Order("id").Find(&msgs).Error; err != nil {
		return nil, errs.Internal(err)
	}
	e := &Export{s: s, ch: ch, contact: contact, msgs: msgs, names: map[uint]string{}}
	for i := range msgs {
		if id := msgs[i].SenderUserID; id != nil {
			if _, ok := e.names[*id]; !ok {
				e.names[*id] = "Temsilci"
				if u, err := s.users.GetByID(ctx, *id); err == nil && u != nil {
					e.names[*id] = u.Name
				}
			}
		}
	}
	if u, err := s.users.GetByID(ctx, actorID); err == nil && u != nil {
		e.by = u.Name
	}
	name := unsafeName.ReplaceAllString(strings.TrimSpace(contactView(contact).Display), "_")
	if name == "" {
		name = contact.WAID
	}
	e.FileName = fmt.Sprintf("whatsapp_%s_%d.zip", name, conv.ID)
	return e, nil
}

type exportDay struct {
	Label string
	Items []exportItem
}

type exportItem struct {
	Event   string
	Side    string // in, out, note
	Who     string
	Color   string // c0..c7, one colour per person
	Time    string
	Text    template.HTML
	Quote   string
	Kind    string // image, video, audio, file
	File    string // path inside the archive
	Name    string
	Size    string
	Missing string // why the file is not in the archive
	Ticks   string // ✓, ✓✓ or ✓✓ read
	Failed  string
}

// Write streams the archive: the files first, then the page, which by then
// knows which files made it in.
func (e *Export) Write(ctx context.Context, w io.Writer) error {
	zw := zip.NewWriter(w)
	cv := contactView(e.contact)
	byWAMID := map[string]*models.WAMessage{}
	for i := range e.msgs {
		if id := e.msgs[i].WAMID; id != nil {
			byWAMID[*id] = &e.msgs[i]
		}
	}
	var (
		days  []exportDay
		total int64
		files int
	)
	for i := range e.msgs {
		m := &e.msgs[i]
		at := m.CreatedAt.In(istanbul)
		label := dayLabel(at)
		if len(days) == 0 || days[len(days)-1].Label != label {
			days = append(days, exportDay{Label: label})
		}
		day := &days[len(days)-1]
		if m.Direction == "event" {
			day.Items = append(day.Items, exportItem{Event: m.Body, Time: at.Format("15:04")})
			continue
		}
		it := exportItem{Side: m.Direction, Time: at.Format("15:04")}
		switch m.Direction {
		case "in":
			it.Who = cv.Display
		case "note":
			it.Who = "İç not"
			if m.SenderUserID != nil {
				it.Who = "İç not · " + e.names[*m.SenderUserID]
			}
		default:
			it.Side = "out"
			switch {
			case m.SenderUserID != nil:
				it.Who = e.names[*m.SenderUserID]
				it.Color = fmt.Sprintf("c%d", *m.SenderUserID%8)
			case m.SenderKind == "bot":
				it.Who = "Chatbot"
			default:
				it.Who = "Otomatik mesaj"
			}
			if m.SenderLabel != "" && m.SenderUserID == nil {
				it.Who += " · " + m.SenderLabel
			}
			switch {
			case m.Status == "failed":
				it.Failed = "Gönderilemedi"
				if m.ErrorText != "" {
					it.Failed += ": " + m.ErrorText
				}
			case m.Status == "read":
				it.Ticks = "read"
			case m.Status == "delivered":
				it.Ticks = "✓✓"
			case m.Status == "sent":
				it.Ticks = "✓"
			}
		}
		if m.ReplyToWAMID != nil {
			if q := byWAMID[*m.ReplyToWAMID]; q != nil {
				it.Quote = shorten(quoteText(q), 140)
			}
		}
		text := strings.TrimSpace(m.Body)
		if m.Kind == "location" && text == "" {
			text = "📍 Konum paylaşıldı"
		}
		it.Text = exportText(text)

		ref := e.s.mediaRef(m)
		if ref.MetaID != "" || ref.StoreID != "" {
			it.Kind = "file"
			switch m.Kind {
			case "image", "sticker":
				it.Kind = "image"
			case "video":
				it.Kind = "video"
			case "audio":
				it.Kind = "audio"
			}
			name := ref.Name
			if name == "" {
				name = fmt.Sprintf("%s-%d%s", m.Kind, m.ID, extFor(ref.Mime))
			}
			it.Name = name
			if ref.Size > 0 {
				it.Size = humanSize(ref.Size)
			}
			if total+ref.Size > exportMediaLimit {
				it.Missing = "Arşiv çok büyüdüğü için eklenmedi."
			} else {
				files++
				file := fmt.Sprintf("medya/%04d_%s", files, safeFile(name))
				n, err := e.copyMedia(ctx, zw, m, ref, file)
				total += n
				if err != nil {
					it.Missing = err.Error()
				} else {
					it.File = file
				}
			}
		}
		day.Items = append(day.Items, it)
	}

	page, err := zw.Create("sohbet.html")
	if err != nil {
		return err
	}
	data := map[string]any{
		"Title":   cv.Display,
		"Initial": initial(cv.Display),
		"Phone":   "+" + e.contact.WAID,
		"Channel": e.ch.Name,
		"By":      e.by,
		"At":      time.Now().In(istanbul).Format("02.01.2006 15:04"),
		"Count":   len(e.msgs),
		"Files":   files,
		"Days":    days,
	}
	if err := exportPage.Execute(page, data); err != nil {
		return err
	}
	return zw.Close()
}

// copyMedia puts one file into the archive, from our storage when it was
// kept there, otherwise from Meta while it still has it.
func (e *Export) copyMedia(ctx context.Context, zw *zip.Writer, m *models.WAMessage, ref MediaRef, file string) (int64, error) {
	var src io.Reader
	if ref.StoreID != "" && e.s.storage != nil {
		c, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		resp, err := e.s.storage.Open(c, ref.StoreID, "")
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode < 300 {
				src = resp.Body
			}
		} else {
			slog.WarnContext(ctx, "whatsapp export: file could not be read from storage", "message", m.ID, "error", err)
		}
	}
	if src == nil && ref.MetaID != "" {
		cl, err := e.s.cloudFor(e.ch)
		if err == nil {
			c, cancel := context.WithTimeout(ctx, 5*time.Minute)
			data, _, derr := cl.Download(c, ref.MetaID, mediaLimit)
			cancel()
			if derr == nil {
				src = bytes.NewReader(data)
			}
		}
	}
	if src == nil {
		return 0, errors.New("Dosyaya artık ulaşılamıyor.")
	}
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: file, Method: zip.Store, Modified: m.CreatedAt})
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(fw, src)
	if err != nil {
		return n, errors.New("Dosya yarıda kaldı.")
	}
	return n, nil
}

func initial(s string) string {
	for _, r := range strings.TrimSpace(s) {
		return strings.ToUpper(string(r))
	}
	return "?"
}

func dayLabel(t time.Time) string {
	months := []string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()-1], t.Year())
}

func safeFile(name string) string {
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	base = strings.Trim(unsafeFile.ReplaceAllString(base, "_"), "_.")
	if r := []rune(base); len(r) > 60 {
		base = string(r[:60])
	}
	if base == "" {
		base = "dosya"
	}
	return base + unsafeFile.ReplaceAllString(ext, "")
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d B", n)
}

func shorten(s string, n int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

func quoteText(m *models.WAMessage) string {
	if t := strings.TrimSpace(m.Body); t != "" {
		return t
	}
	return "[" + kindWord(m.Kind) + "]"
}

// exportText turns WhatsApp's *bold*, _italic_ and ~strike~ into HTML, with
// everything else escaped.
var (
	waBold   = regexp.MustCompile(`\*([^*\n]+)\*`)
	waItalic = regexp.MustCompile(`(^|[\s(])_([^_\n]+)_`)
	waStrike = regexp.MustCompile(`~([^~\n]+)~`)
	waLink   = regexp.MustCompile(`https?://[^\s<]+`)
)

func exportText(s string) template.HTML {
	h := template.HTMLEscapeString(s)
	h = waLink.ReplaceAllStringFunc(h, func(u string) string { return `<a href="` + u + `" target="_blank" rel="noopener">` + u + `</a>` })
	h = waBold.ReplaceAllString(h, "<b>$1</b>")
	h = waItalic.ReplaceAllString(h, "$1<i>$2</i>")
	h = waStrike.ReplaceAllString(h, "<s>$1</s>")
	return template.HTML(h)
}

var exportPage = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="tr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} · WhatsApp yazışması</title>
<style>
:root{--wall:#efeae2;--in:#fff;--out:#d9fdd3;--note:#fff3c4;--text:#111b21;--meta:#667781;--bar:#f0f2f5;--line:#e9edef;--accent:#008069;--event:#ffffffd9}
@media (prefers-color-scheme:dark){:root{--wall:#0b141a;--in:#202c33;--out:#005c4b;--note:#3d3520;--text:#e9edef;--meta:#8696a0;--bar:#202c33;--line:#2a3942;--accent:#00a884;--event:#182229}}
*{box-sizing:border-box}
body{margin:0;background:var(--wall);color:var(--text);font:15px/1.4 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
header{position:sticky;top:0;z-index:2;display:flex;align-items:center;gap:14px;padding:12px 20px;background:var(--bar);border-bottom:1px solid var(--line)}
.av{flex:none;width:44px;height:44px;border-radius:50%;background:var(--accent);color:#fff;display:flex;align-items:center;justify-content:center;font-weight:600;font-size:18px}
header h1{margin:0;font-size:17px;font-weight:600}
header p{margin:2px 0 0;font-size:12.5px;color:var(--meta)}
main{max-width:860px;margin:0 auto;padding:12px 16px 40px}
.day{position:sticky;top:76px;z-index:1;display:flex;justify-content:center;margin:14px 0 8px}
.day span,.ev span{background:var(--event);color:var(--meta);font-size:12px;padding:5px 12px;border-radius:8px;box-shadow:0 1px .5px #0000001a}
.ev{display:flex;justify-content:center;margin:6px 0;text-align:center}
.row{display:flex;margin:2px 0}
.row.out{justify-content:flex-end}
.row.note{justify-content:center}
.b{position:relative;max-width:min(560px,80%);padding:6px 9px 8px;border-radius:8px;background:var(--in);box-shadow:0 1px .5px #0000001f;word-wrap:break-word;overflow-wrap:anywhere}
.out .b{background:var(--out)}
.note .b{background:var(--note);max-width:min(620px,90%)}
.who{font-size:12.8px;font-weight:600;margin-bottom:2px;color:var(--accent)}
.note .who{color:#b45309}
.c0{color:#1f7aec}.c1{color:#d94c8a}.c2{color:#c2410c}.c3{color:#7c3aed}.c4{color:#0f766e}.c5{color:#a16207}.c6{color:#be123c}.c7{color:#15803d}
@media (prefers-color-scheme:dark){.c0{color:#53bdeb}.c1{color:#ff72a1}.c2{color:#fc9775}.c3{color:#a791ff}.c4{color:#5eead4}.c5{color:#ffd279}.c6{color:#ff8a9a}.c7{color:#7fd48f}.note .who{color:#fbbf24}}
.t{white-space:pre-wrap}
.t a{color:#027eb5}
.m{float:right;margin:6px 0 -4px 12px;font-size:11px;color:var(--meta);white-space:nowrap}
.m .r{color:#53bdeb}
.q{border-left:4px solid var(--accent);background:#0000000d;border-radius:6px;padding:4px 8px;margin:2px 0 6px;font-size:13px;color:var(--meta)}
.img{display:block;margin:-2px -5px 4px;border-radius:6px;overflow:hidden}
.img img{display:block;max-width:100%;max-height:420px}
video{display:block;max-width:100%;max-height:420px;border-radius:6px;margin:-2px -5px 4px}
audio{display:block;width:280px;max-width:100%;margin:2px 0 4px}
.doc{display:flex;align-items:center;gap:10px;padding:8px 10px;margin:0 0 4px;border-radius:6px;background:#0000000d;color:inherit;text-decoration:none}
.doc i{flex:none;width:34px;height:40px;border-radius:4px;background:var(--accent);color:#fff;font:600 10px/40px system-ui;text-align:center;font-style:normal}
.doc b{display:block;font-weight:500;font-size:14px}
.doc small{color:var(--meta)}
.miss{font-size:12.5px;color:var(--meta);font-style:italic;margin-bottom:4px}
.fail{font-size:12px;color:#dc2626;margin-top:4px}
footer{text-align:center;color:var(--meta);font-size:12px;padding:10px 0 30px}
</style>
</head>
<body>
<header>
<div class="av">{{.Initial}}</div>
<div>
<h1>{{.Title}}</h1>
<p>{{.Phone}} · {{.Channel}} · {{.Count}} mesaj, {{.Files}} dosya</p>
</div>
</header>
<main>
{{range .Days}}
<div class="day"><span>{{.Label}}</span></div>
{{range .Items}}
{{if .Event}}<div class="ev"><span>{{.Event}} · {{.Time}}</span></div>{{else}}
<div class="row {{.Side}}"><div class="b">
<div class="who {{.Color}}">{{.Who}}</div>
{{if .Quote}}<div class="q">{{.Quote}}</div>{{end}}
{{if .File}}
{{if eq .Kind "image"}}<a class="img" href="{{.File}}" target="_blank"><img src="{{.File}}" alt="{{.Name}}" loading="lazy"></a>
{{else if eq .Kind "video"}}<video src="{{.File}}" controls preload="metadata"></video>
{{else if eq .Kind "audio"}}<audio src="{{.File}}" controls preload="none"></audio>
{{else}}<a class="doc" href="{{.File}}" target="_blank"><i>DOSYA</i><span><b>{{.Name}}</b><small>{{.Size}}</small></span></a>{{end}}
{{else if .Name}}<div class="miss">{{.Name}}: {{.Missing}}</div>{{end}}
{{if .Text}}<span class="t">{{.Text}}</span>{{end}}
<span class="m">{{.Time}}{{if eq .Ticks "read"}} <span class="r">✓✓</span>{{else if .Ticks}} {{.Ticks}}{{end}}</span>
{{if .Failed}}<div class="fail">{{.Failed}}</div>{{end}}
<div style="clear:both"></div>
</div></div>
{{end}}
{{end}}
{{end}}
</main>
<footer>{{.At}} tarihinde{{if .By}} {{.By}} tarafından{{end}} dışa aktarıldı. Dosyalar "medya" klasöründe.</footer>
</body>
</html>
`))
