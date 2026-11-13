package apitester

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

var allowedMethods = map[string]bool{
	"GET":     true,
	"POST":    true,
	"PUT":     true,
	"PATCH":   true,
	"DELETE":  true,
	"HEAD":    true,
	"OPTIONS": true,
}

var varPlaceholderRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)

// NormalizeMethod validates and upper-cases an HTTP method name.
func NormalizeMethod(m string) (string, error) {
	m = strings.ToUpper(strings.TrimSpace(m))
	if m == "" {
		m = "GET"
	}
	if !allowedMethods[m] {
		return "", errs.Invalid("Desteklenmeyen HTTP metodu.", nil)
	}
	return m, nil
}

// ValidateURLTemplate checks that a URL template uses http/https or starts with
// a variable placeholder (such as {{base_url}}/...).
func ValidateURLTemplate(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errs.Invalid("URL şablonu boş olamaz.", nil)
	}
	if len(s) > 4000 {
		return "", errs.Invalid("URL şablonu çok uzun.", nil)
	}
	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(s, "{{") {
		return "", errs.Invalid("URL http://, https:// veya {{base_url}} gibi bir değişkenle başlamalıdır.", nil)
	}
	return s, nil
}

// ValidateCollectionBaseURL checks an optional collection base URL.
func ValidateCollectionBaseURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errs.Invalid("Koleksiyon taban URL adresi geçerli bir http(s) adresi olmalıdır.", err)
	}
	return strings.TrimRight(s, "/"), nil
}

// Interpolate substitutes {{key}} placeholders in tmpl using vars.
func Interpolate(tmpl string, vars map[string]string) string {
	if tmpl == "" || len(vars) == 0 {
		return tmpl
	}
	return varPlaceholderRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		sub := varPlaceholderRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		if v, ok := vars[sub[1]]; ok {
			return v
		}
		return match
	})
}

// BuildVars merges collection baseUrl, variable defaults and caller values.
func BuildVars(baseURL string, defs []VariableDef, supplied map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(defs)+len(supplied)+1)
	if strings.TrimSpace(baseURL) != "" {
		out["base_url"] = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
	for _, d := range defs {
		k := strings.TrimSpace(d.Key)
		if k == "" {
			continue
		}
		if d.DefaultValue != "" {
			out[k] = d.DefaultValue
		}
	}
	for k, v := range supplied {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = v
	}
	for _, d := range defs {
		k := strings.TrimSpace(d.Key)
		if k == "" || !d.Required {
			continue
		}
		if strings.TrimSpace(out[k]) == "" {
			label := strings.TrimSpace(d.Label)
			if label == "" {
				label = k
			}
			return nil, errs.Invalid(fmt.Sprintf("%s alanı zorunludur.", label), nil)
		}
	}
	return out, nil
}

// RenderTarget resolves urlTemplate + queryParams with vars and validates the
// final http(s) target URL.
func RenderTarget(urlTemplate string, queryParams []KeyValue, vars map[string]string) (string, error) {
	rendered := strings.TrimSpace(Interpolate(urlTemplate, vars))
	u, err := url.Parse(rendered)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errs.Invalid("Oluşturulan hedef URL geçersiz: değişkenleri veya taban adresi kontrol edin.", err)
	}
	if len(queryParams) > 0 {
		q := u.Query()
		for _, p := range queryParams {
			k := strings.TrimSpace(p.Key)
			if k == "" {
				continue
			}
			v := Interpolate(p.Value, vars)
			q.Add(k, v)
		}
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

// RedactLogURL strips query parameter values so secrets passed via query params
// are never persisted in execution logs.
func RedactLogURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		if len(raw) > 250 {
			return raw[:250]
		}
		return raw
	}
	if u.RawQuery != "" {
		q := u.Query()
		redacted := url.Values{}
		for k := range q {
			redacted.Set(k, "***")
		}
		u.RawQuery = redacted.Encode()
	}
	u.User = nil
	out := u.String()
	if len(out) > 500 {
		return out[:500]
	}
	return out
}
