package whatsapp

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

// Values a chatbot collected come from customers, so they are escaped for
// the place they land in an outside request: a path segment, a query value,
// a JSON string or a header. They may never choose the server: a {name} in
// the scheme or host part of an address is refused.

var errVarInHost = errors.New("adresin sunucu kısmında değişken kullanılamaz; değişkenler yalnızca yolda ve sorguda olabilir")

// checkURLTemplate refuses an address whose scheme or host contains a
// variable.
func checkURLTemplate(raw string) error {
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

// fillURL puts vars into an address: path-escaped before the "?", and
// query-escaped after it.
func fillURL(raw string, vars map[string]string) (string, error) {
	if err := checkURLTemplate(raw); err != nil {
		return "", err
	}
	path, query, hasQuery := strings.Cut(raw, "?")
	out := fillVarsWith(path, vars, url.PathEscape)
	if hasQuery {
		out += "?" + fillVarsWith(query, vars, url.QueryEscape)
	}
	return out, nil
}

// fillJSON puts vars into a JSON body as escaped string content, so a quote
// or a backslash in an answer cannot break out of the string it is in.
func fillJSON(body string, vars map[string]string) string {
	return fillVarsWith(body, vars, func(v string) string {
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b[1 : len(b)-1])
	})
}

// fillHeader puts vars into a header value with line breaks removed, so an
// answer cannot add a header of its own.
func fillHeader(value string, vars map[string]string) string {
	return fillVarsWith(value, vars, func(v string) string {
		return strings.NewReplacer("\r", " ", "\n", " ").Replace(v)
	})
}

// fillVarsWith is fillVars with each value passed through escape.
func fillVarsWith(text string, vars map[string]string, escape func(string) string) string {
	return varRef.ReplaceAllStringFunc(text, func(m string) string {
		if v, ok := vars[strings.Trim(m, "{}")]; ok {
			return escape(v)
		}
		return m
	})
}
