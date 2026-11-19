package record

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	phonePattern = regexp.MustCompile(`^\+?[0-9][0-9 ()-]{4,28}$`)
	webPattern   = regexp.MustCompile(`^https?://[^\s/$.?#][^\s]*$`)
)

// OptionalEmail is an email address, lower-cased; nil when blank.
func OptionalEmail(v Values, key, label string, p *Problems) *string {
	s := OptionalText(v, key, label, 150, p)
	if s == nil || p.Has(key) {
		return nil
	}
	e := strings.ToLower(*s)
	if !emailPattern.MatchString(e) {
		p.Add(key, fmt.Sprintf("%s bukan alamat yang sah, mis. nama@perusahaan.co.id.", label))
		return nil
	}
	return &e
}

// OptionalPhone is a phone number: digits with spaces, ( ) and -, optionally starting with +; nil when blank.
func OptionalPhone(v Values, key, label string, p *Problems) *string {
	s := OptionalText(v, key, label, 30, p)
	if s != nil && !p.Has(key) && !phonePattern.MatchString(*s) {
		p.Add(key, fmt.Sprintf("%s berisi angka, spasi, ( ) atau -, boleh diawali +, mis. +62 21 555 0101.", label))
		return nil
	}
	return s
}

// OptionalURL is a web address starting with http:// or https://; nil when blank.
func OptionalURL(v Values, key, label string, max int, p *Problems) *string {
	s := OptionalText(v, key, label, max, p)
	if s != nil && !p.Has(key) && !webPattern.MatchString(*s) {
		p.Add(key, fmt.Sprintf("%s diawali http:// atau https://, mis. https://perusahaan.co.id.", label))
		return nil
	}
	return s
}

// Opt is s as a form value (nil when nil).
func Opt[T any](s *T) any {
	if s == nil {
		return nil
	}
	return *s
}
