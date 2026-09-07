// Package sms knows how a text is carried on the air: which alphabet it
// needs and how many segments the radio splits it into. Android meters an
// app's outgoing texts by segment, so the count is what pacing is about, and
// it is what the estimate of when a send will be done rests on.
package sms

import "unicode/utf16"

// The GSM 03.38 default alphabet. Each of these is one septet.
const gsmBasic = "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà"

// The extension table. Each of these costs two septets: an escape and the
// character.
const gsmExtension = "\f^{}\\[~]|€"

const (
	gsmSingle = 160 // septets that fit in one segment
	gsmMulti  = 153 // septets per segment once a concatenation header is needed
	ucsSingle = 70  // UTF-16 units that fit in one segment
	ucsMulti  = 67
)

var (
	basic     = runeSet(gsmBasic)
	extension = runeSet(gsmExtension)
)

func runeSet(s string) map[rune]struct{} {
	m := make(map[rune]struct{}, len(s))
	for _, r := range s {
		m[r] = struct{}{}
	}
	return m
}

// Septets returns how many GSM-7 septets the text needs, and false when it
// holds a character the alphabet cannot carry, which forces UCS-2 for the
// whole text.
func Septets(body string) (int, bool) {
	n := 0
	for _, r := range body {
		switch {
		case isBasic(r):
			n++
		case isExtension(r):
			n += 2
		default:
			return 0, false
		}
	}
	return n, true
}

func isBasic(r rune) bool {
	_, ok := basic[r]
	return ok
}

func isExtension(r rune) bool {
	_, ok := extension[r]
	return ok
}

// Unicode reports whether the text needs UCS-2, which halves what a segment
// holds. Turkish dotless i, ş and ğ are not in the GSM alphabet, so most
// Turkish text does.
func Unicode(body string) bool {
	_, gsm := Septets(body)
	return !gsm
}

// Segments is how many segments the text takes on the air: one while it
// fits, otherwise however many concatenated parts it needs. An empty text
// takes none.
func Segments(body string) int {
	if body == "" {
		return 0
	}
	units, single, multi := 0, ucsSingle, ucsMulti
	if septets, gsm := Septets(body); gsm {
		units, single, multi = septets, gsmSingle, gsmMulti
	} else {
		for _, r := range body {
			units += utf16.RuneLen(r)
		}
	}
	if units <= single {
		return 1
	}
	return (units + multi - 1) / multi
}
