package googledoc

import (
	"strings"

	"google.golang.org/api/docs/v1"
)

// determine whether a font is monospace (used for detecting code blocks)
func IsMonospace(font *docs.WeightedFontFamily) bool {
	if font == nil {
		return false
	}

	switch strings.ToLower(font.FontFamily) {
	case "courier new":
		return true
	case "consolas":
		return true
	case "roboto mono":
		return true
	}
	return false
}
