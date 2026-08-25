package fiox

import (
	"fmt"
	"io"
	"strings"
)

type ReadFunc func(r io.Reader, v any) error
type WriteFunc func(w io.Writer, data any) error

type Directive struct {
	Read  ReadFunc
	Write WriteFunc
}

func register(ext string, read ReadFunc, write WriteFunc) {
	ext = strings.ToLower(ext)
	if ext == "" {
		panic("tried to register empty ext")
	}

	if !strings.HasPrefix(ext, ".") {
		panic(fmt.Sprintf("ext [%s] has no leading dot", ext))
	}

	if _, ok := directives[ext]; ok {
		panic(fmt.Sprintf("tried to register ext [%s] more than once", ext))
	}

	directives[ext] = Directive{
		Read:  read,
		Write: write,
	}
}
