package fiox

import (
	"encoding/json"
	"fmt"
	"io"
)

func init() {
	read := func(r io.Reader, v any) error {
		if err := json.NewDecoder(r).Decode(v); err != nil {
			return fmt.Errorf("json decode error: %w", err)
		}

		return nil
	}

	write := func(w io.Writer, data any) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "    ")
		if err := enc.Encode(data); err != nil {
			return fmt.Errorf("json encode error: %w", err)
		}

		return nil
	}

	register(".json", read, write)
}
