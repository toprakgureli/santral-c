package middlewares

import "testing"

func TestAddressGroup(t *testing.T) {
	for in, want := range map[string]string{
		"198.51.100.7":                  "198.51.100.7",
		"2001:db8:1:2:aaaa:bbbb:cccc:1": "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::9":          "2001:db8:1:2::/64",
		"not-an-address":                "not-an-address",
	} {
		if got := addressGroup(in); got != want {
			t.Errorf("addressGroup(%q) = %q, want %q", in, got, want)
		}
	}
}
