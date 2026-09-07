package sms

import (
	"strings"
	"testing"
)

func TestSegments(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		want    int
		unicode bool
	}{
		{"empty", "", 0, false},
		{"one word", "hello", 1, false},
		{"exactly one GSM segment", strings.Repeat("a", 160), 1, false},
		{"one over the GSM segment", strings.Repeat("a", 161), 2, false},
		{"three GSM segments", strings.Repeat("a", 154*2+1), 3, false},
		{"extension characters cost two septets", strings.Repeat("a", 158) + "{", 1, false},
		{"an extension character tips it over", strings.Repeat("a", 159) + "{", 2, false},
		{"the euro sign is an extension character", strings.Repeat("a", 159) + "€", 2, false},
		{"GSM accents stay GSM", "café à Zürich, señor", 1, false},
		{"Turkish forces UCS-2", "ışık", 1, true},
		{"exactly one UCS-2 segment", strings.Repeat("ş", 70), 1, true},
		{"one over the UCS-2 segment", strings.Repeat("ş", 71), 2, true},
		{"268 Turkish characters are four segments", strings.Repeat("ğ", 268), 4, true},
		{"270 Turkish characters are five", strings.Repeat("ğ", 270), 5, true},
		{"an emoji is two UTF-16 units", strings.Repeat("a", 68) + "😀", 1, true},
		{"an emoji tips it over", strings.Repeat("a", 69) + "😀", 2, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Segments(c.body); got != c.want {
				t.Errorf("Segments = %d, want %d", got, c.want)
			}
			if got := Unicode(c.body); got != c.unicode {
				t.Errorf("Unicode = %v, want %v", got, c.unicode)
			}
		})
	}
}
