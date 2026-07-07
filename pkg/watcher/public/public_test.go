package public

import "testing"

func TestIPv4RegexExtractsFromText(t *testing.T) {
	cases := map[string]string{
		"1.2.3.4": "1.2.3.4",
		"当前 IP：203.0.113.9  来自于：中国 北京 电信":    "203.0.113.9",
		"Current IP Address: 198.51.100.7\n": "198.51.100.7",
		"no ip here":                         "",
	}
	for body, want := range cases {
		if got := ipv4Regex.FindString(body); got != want {
			t.Errorf("body %q: got %q, want %q", body, got, want)
		}
	}
}
