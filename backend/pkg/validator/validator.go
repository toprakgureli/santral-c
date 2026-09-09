// Package validator validates request DTOs.
package validator

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"

	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

var (
	validate *validator.Validate
	once     sync.Once
)

// Get returns the shared validator.
func Get() *validator.Validate {
	once.Do(func() {
		validate = validator.New(validator.WithRequiredStructEnabled())
	})
	return validate
}

// Struct validates a struct and returns a typed invalid error on failure.
func Struct(s any) error {
	err := Get().Struct(s)
	if err == nil {
		return nil
	}
	var invalid validator.ValidationErrors
	if !errors.As(err, &invalid) {
		return errs.Invalid("Gönderilen veri doğrulanamadı.", err)
	}
	fields := make([]string, 0, len(invalid))
	for _, e := range invalid {
		fields = append(fields, e.Field())
	}
	return errs.Invalid(fmt.Sprintf("Geçersiz alan: %s", strings.Join(fields, ", ")), err)
}
