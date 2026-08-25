package fiox

import (
	"fmt"
	"io"
)

func init() {
	read := func(r io.Reader, v any) error {
		bytes, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("read file error: %w", err)
		}

		switch vt := v.(type) {
		case *string:
			*vt = string(bytes)
		case *[]byte:
			*vt = bytes
		default:
			return fmt.Errorf("v must be *string or *[]byte")
		}

		return nil
	}

	write := func(w io.Writer, data any) error {
		var toWrite []byte
		switch d := data.(type) {
		case string:
			toWrite = []byte(d)
		case []byte:
			toWrite = d
		default:
			toWrite = fmt.Appendf(toWrite, "%v", data)
		}

		if _, err := w.Write(toWrite); err != nil {
			return fmt.Errorf("write file error: %w", err)
		}

		return nil
	}

	register(".txt", read, write)
}
