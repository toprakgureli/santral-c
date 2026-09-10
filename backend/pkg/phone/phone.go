// Package phone normalizes dialed numbers to E.164 so a contact resolves to a
// single canonical form regardless of how the number was entered.
package phone

import (
	"errors"
	"strings"
)

// ErrInvalid reports a number that cannot be normalized to E.164.
var ErrInvalid = errors.New("number could not be normalized to E.164")

const defaultCountry = "90" // Turkey

// Key reduces any dialed form of a number to a stable lookup key: the last ten
// digits (so 0530..., 90530..., +90530... and 530... all match), or the full
// digit string for short internal numbers. It never errors.
func Key(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	digits := b.String()
	if len(digits) > 10 {
		return digits[len(digits)-10:]
	}
	return digits
}

// Normalize converts a raw number to E.164 (a leading + and 8-15 digits).
// Turkish local forms (0XXXXXXXXXX and bare 10-digit) assume the +90 country
// code; already-international forms are preserved.
func Normalize(raw string) (string, error) {
	var b strings.Builder
	plus := false
	for i, r := range raw {
		switch {
		case r == '+' && i == 0:
			plus = true
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.' || r == '/':
			// separators are ignored
		default:
			return "", ErrInvalid
		}
	}
	digits := b.String()

	switch {
	case plus:
		// already international
	case strings.HasPrefix(digits, "00"):
		digits = strings.TrimPrefix(digits, "00")
	case strings.HasPrefix(digits, "0") && len(digits) == 11:
		digits = defaultCountry + digits[1:]
	case len(digits) == 10:
		digits = defaultCountry + digits
	case strings.HasPrefix(digits, defaultCountry) && len(digits) == 12:
		// bare country code without plus
	default:
		return "", ErrInvalid
	}

	if len(digits) < 8 || len(digits) > 15 {
		return "", ErrInvalid
	}
	return "+" + digits, nil
}
