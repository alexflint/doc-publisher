package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCommandPattern(t *testing.T) {
	s := `\newcommand{\foo}{bar}` + "\n"
	var cmd newcommand
	if assert.True(t, newcommandPattern.Find(&cmd, s)) {
		assert.Equal(t, `\foo`, cmd.Name)
		assert.Equal(t, `bar`, cmd.Value)
	}
}
