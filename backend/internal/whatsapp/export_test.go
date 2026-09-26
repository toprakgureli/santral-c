package whatsapp

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

func TestExportPage(t *testing.T) {
	uid := uint(3)
	wamid := "wamid.1"
	at := time.Date(2026, 9, 25, 14, 5, 0, 0, istanbul)
	e := &Export{
		s:       &Service{},
		ch:      &models.WAChannel{Name: "Destek"},
		contact: &models.WAContact{WAID: "905551112233", Name: "Şule Kaya"},
		names:   map[uint]string{uid: "Toprak Gureli"},
		msgs: []models.WAMessage{
			{ID: 1, Direction: "event", Body: "Sohbet #4 açıldı", CreatedAt: at},
			{ID: 2, Direction: "in", Kind: "text", Body: "Merhaba, *faturam* gelmedi <script>", WAMID: &wamid, CreatedAt: at},
			{ID: 3, Direction: "out", Kind: "text", Body: "Hemen bakıyorum https://ornek.com", SenderUserID: &uid, Status: "read", ReplyToWAMID: &wamid, CreatedAt: at.Add(time.Minute)},
			{ID: 4, Direction: "note", Kind: "text", Body: "Muhasebeye sordum", SenderUserID: &uid, CreatedAt: at.Add(24 * time.Hour)},
		},
	}
	var buf bytes.Buffer
	if err := e.Write(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 1 || zr.File[0].Name != "sohbet.html" {
		t.Fatalf("unexpected files %v", zr.File)
	}
	f, _ := zr.File[0].Open()
	page, _ := io.ReadAll(f)
	html := string(page)
	for _, want := range []string{"Şule Kaya", "<b>faturam</b>", "&lt;script&gt;", "Toprak Gureli", `class="q"`, "26 Eylül 2026", `href="https://ornek.com"`, `class="av">Ş<`} {
		if !strings.Contains(html, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if p := os.Getenv("EXPORT_PREVIEW"); p != "" {
		_ = os.WriteFile(p, page, 0o644)
	}
}
