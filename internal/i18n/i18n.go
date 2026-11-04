// Package i18n picks between the French and English text of cairn's interface.
//
// Every user-facing string is written in place as T("français", "English"),
// so a missing translation cannot exist and the two stay side by side.
package i18n

import "sync/atomic"

var english atomic.Bool

// Set chooses the language: "en" for English, anything else for French.
func Set(lang string) { english.Store(lang == "en") }

// Lang is "en" or "fr".
func Lang() string {
	if english.Load() {
		return "en"
	}
	return "fr"
}

// English reports whether the interface is in English.
func English() bool { return english.Load() }

// T returns the text in the current language.
func T(fr, en string) string {
	if english.Load() {
		return en
	}
	return fr
}
