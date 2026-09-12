// Package password holds the account password policy shared by every place a
// password is set: 8 to 16 characters with an upper-case letter, a lower-case
// letter, a digit and a special character.
package password

import (
	"unicode"
	"unicode/utf8"

	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Length bounds of the policy.
const (
	MinLength = 8
	MaxLength = 16
)

// Validate returns a typed invalid error when the password breaks the policy.
func Validate(pw string) error {
	n := utf8.RuneCountInString(pw)
	if n < MinLength || n > MaxLength {
		return errs.Invalid("Şifre 8 ile 16 karakter arasında olmalı.", nil)
	}
	var upper, lower, digit, special bool
	for _, r := range pw {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			special = true
		}
	}
	if !upper || !lower || !digit || !special {
		return errs.Invalid("Şifre büyük harf, küçük harf, rakam ve özel karakter içermeli.", nil)
	}
	return nil
}
