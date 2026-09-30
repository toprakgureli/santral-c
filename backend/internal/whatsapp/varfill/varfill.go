// Package varfill fills {name} placeholders in WhatsApp texts: chatbot
// messages, automatic messages and outside-system requests.
package varfill

import (
	"regexp"
	"strings"
)

// Pattern matches a placeholder: a name in braces, Turkish letters included.
var Pattern = regexp.MustCompile(`\{([a-zA-Z0-9_ğüşıöçĞÜŞİÖÇ]+)\}`)

// Fill replaces each known {name} with its value and leaves unknown ones.
func Fill(text string, vars map[string]string) string {
	return FillWith(text, vars, func(v string) string { return v })
}

// FillWith is Fill with each value passed through escape first.
func FillWith(text string, vars map[string]string, escape func(string) string) string {
	return Pattern.ReplaceAllStringFunc(text, func(m string) string {
		if v, ok := vars[strings.Trim(m, "{}")]; ok {
			return escape(v)
		}
		return m
	})
}
