package phone

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"local zero prefix", "0555 111 22 33", "+905551112233"},
		{"bare ten digits", "5551112233", "+905551112233"},
		{"international plus", "+90 555 111 22 33", "+905551112233"},
		{"double zero prefix", "0090 555 111 22 33", "+905551112233"},
		{"bare country code", "905551112233", "+905551112233"},
		{"foreign number", "+1 (415) 555-2671", "+14155552671"},
		{"punctuation", "+90.555.111.22.33", "+905551112233"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Normalize(c.in)
			if err != nil {
				t.Fatalf("Normalize(%q) unexpected error: %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("Normalize(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizeInvalid(t *testing.T) {
	cases := []string{"", "abc", "123", "0532111", "+", "++90"}
	for _, in := range cases {
		if _, err := Normalize(in); err == nil {
			t.Fatalf("Normalize(%q) expected error, got nil", in)
		}
	}
}
