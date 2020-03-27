package stringbuilder

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

type B struct {
	// const
	indentMultiplier int

	// mut
	sb     *strings.Builder
	indent int
	width  int
	x, y   int
}

type Config struct {
	IndentMultiplier int `validate:"gte=1"`
	Width            int `validate:"gte=1"`
}

var validate = validator.New()

func New(config Config) *B {

	if err := validate.Struct(config); err != nil {
		panic(err)
	}

	return &B{sb: &strings.Builder{}, width: config.Width, indentMultiplier: config.IndentMultiplier}
}

func (b *B) String() string { return b.sb.String() }

func (w *B) Newline() {
	w.Write("\n")
}

func wrap(s string, width int) string {
	var b strings.Builder
	for len(s) > 0 {
		rem := width
		if rem > len(s) {
			rem = len(s)
		}
		if idx := strings.IndexAny(s, "\n\r"); idx != -1 && idx < rem {
			rem = idx + 1
		}
		untilNewline := strings.TrimRight(s[:rem], "\n\r")
		s = s[rem:]
		if len(untilNewline) == 0 {
			continue
		}
		b.WriteString(untilNewline)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n\r")
}

func (w *B) PrintfDrawIndentedAndWrappedIfMultiline(format string, args ...interface{}) {
	whole := fmt.Sprintf(format, args...)
	if strings.ContainsAny(whole, "\n\r") {
		w.AddIndent(1)
		defer w.AddIndent(-1)
	}
	w.Write(whole)
}

func (w *B) Printf(format string, args ...interface{}) {
	whole := fmt.Sprintf(format, args...)
	w.Write(whole)
}

func (t *B) AddIndent(delta int) {
	t.indent += delta * t.indentMultiplier
}

func (t *B) AddIndentAndNewline(delta int) {
	t.indent += delta * t.indentMultiplier
	t.Write("\n")
}

func (w *B) Write(s string) {
	for _, c := range s {
		if c == '\n' {
			fmt.Fprint(w.sb, "\n")
			w.x = 0
			fmt.Fprintf(w.sb, Times(" ", w.indent-w.x))
			w.x = w.indent
			w.y++
			continue
		}
		if w.x >= w.width {
			fmt.Fprint(w.sb, "\n")
			w.x = 0
			fmt.Fprintf(w.sb, Times(" ", w.indent-w.x))
			w.x = w.indent
		}
		fmt.Fprintf(w.sb, "%c", c)
		w.x++
	}
}

func Times(str string, n int) (out string) {
	for i := 0; i < n; i++ {
		out += str
	}
	return
}

func RightPad(str string, length int, pad string) string {
	if len(str) > length {
		return str[:length]
	}
	return str + strings.Repeat(pad, length-len(str))
}

// changeCount = 0 indicates stall / no progresss
func (w *B) DrawBar(length int, bytes, totalBytes int64, changeCount int) {
	const arrowPositions = `>\|/`
	var completedLength int
	if totalBytes > 0 {
		completedLength = int(int64(length) * bytes / totalBytes)
		if completedLength > length {
			completedLength = length
		}
	} else if totalBytes == bytes {
		completedLength = length
	}

	w.Write("[")
	w.Write(Times("=", completedLength))
	w.Write(string(arrowPositions[changeCount%len(arrowPositions)]))
	w.Write(Times("-", length-completedLength))
	w.Write("]")
}
