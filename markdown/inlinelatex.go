package markdown

// This file contains utilities for generating inline latex within markdown

import (
	"strings"
	"unicode"

	"github.com/alexflint/go-restructure"
)

// a regular expression for latex \newcommand lines, which get moved automatically to the latex header
type newcommand struct {
	_     string `^`
	_     string `\\newcommand\{`
	Name  string `.+`
	_     string `\}\{`
	Value string `.*`
	_     string `\}`
}

var newcommandPattern = restructure.MustCompile(&newcommand{}, restructure.Options{})

// splitSpace splits a string into leading whitespace, trailing
// whitespace, and everything inbetween
func splitSpace(s string) (left, middle, right string) {
	for _, r := range s {
		if unicode.IsSpace(r) {
			right += string(r)
		} else if len(middle) == 0 {
			left = right
			right = ""
			middle += string(r)
		} else {
			middle += right + string(r)
			right = ""
		}
	}
	return
}

// fixLatexSymbol changes \T1 to \Tone and so forth, because latex does not permit numbers in symbols
func fixLatexSymbol(s string) string {
	s = strings.ReplaceAll(s, "0", "zero")
	s = strings.ReplaceAll(s, "1", "one")
	s = strings.ReplaceAll(s, "2", "two")
	s = strings.ReplaceAll(s, "3", "three")
	s = strings.ReplaceAll(s, "4", "four")
	s = strings.ReplaceAll(s, "5", "five")
	s = strings.ReplaceAll(s, "6", "six")
	s = strings.ReplaceAll(s, "7", "seven")
	s = strings.ReplaceAll(s, "8", "eight")
	s = strings.ReplaceAll(s, "9", "nine")
	return s
}
