package outside

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/varfill"
)

// Values a chatbot collected come from customers, so they are escaped for
// the place they land in an outside request: a path segment, a query value,
// a JSON string or a header. They may never choose the server: a {name} in
// the scheme or host part of an address is refused.

var errVarInHost = errors.New("adresin sunucu kısmında değişken kullanılamaz; değişkenler yalnızca yolda ve sorguda olabilir")

// CheckURLTemplate refuses an address whose scheme or host contains a
// variable.
func CheckURLTemplate(raw string) error {
	rest := raw
	if i := strings.Index(rest, "://"); i >= 0 {
		if strings.Contains(rest[:i], "{") {
			return errVarInHost
		}
		rest = rest[i+3:]
	}
	if end := strings.IndexAny(rest, "/?#"); end >= 0 {
		rest = rest[:end]
	}
	if strings.Contains(rest, "{") {
		return errVarInHost
	}
	return nil
}

// FillURL puts vars into an address: path-escaped before the "?", and
// query-escaped after it.
func FillURL(raw string, vars map[string]string) (string, error) {
	if err := CheckURLTemplate(raw); err != nil {
		return "", err
	}
	path, query, hasQuery := strings.Cut(raw, "?")
	out := varfill.FillWith(path, vars, url.PathEscape)
	if hasQuery {
		out += "?" + varfill.FillWith(query, vars, url.QueryEscape)
	}
	return out, nil
}

// FillJSON puts vars into a JSON body as escaped string content, so a quote
// or a backslash in an answer cannot break out of the string it is in.
func FillJSON(body string, vars map[string]string) string {
	return varfill.FillWith(body, vars, func(v string) string {
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b[1 : len(b)-1])
	})
}

// FillHeader puts vars into a header value with line breaks removed, so an
// answer cannot add a header of its own.
func FillHeader(value string, vars map[string]string) string {
	return varfill.FillWith(value, vars, func(v string) string {
		return strings.NewReplacer("\r", " ", "\n", " ").Replace(v)
	})
}
