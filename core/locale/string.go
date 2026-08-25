package locale

import (
	"fmt"
	"strconv"
)

type ResolvableString string

func (s ResolvableString) Resolve(code LangCode) string {
	return string(s)
}

func String(s any) ResolvableString {
	if str, ok := s.(string); ok {
		return ResolvableString(str)
	}

	return stringOtherTypes(s)
}

func stringOtherTypes(s any) ResolvableString {
	switch v := s.(type) {
	case int:
		return ResolvableString(strconv.Itoa(v))
	case int64:
		return ResolvableString(strconv.FormatInt(v, 10))
	case int32:
		return ResolvableString(strconv.FormatInt(int64(v), 10))
	case uint:
		return ResolvableString(strconv.FormatUint(uint64(v), 10))
	case uint64:
		return ResolvableString(strconv.FormatUint(v, 10))
	case uint32:
		return ResolvableString(strconv.FormatUint(uint64(v), 10))
	case float64:
		return ResolvableString(strconv.FormatFloat(v, 'f', -1, 64))
	case float32:
		return ResolvableString(strconv.FormatFloat(float64(v), 'f', -1, 32))
	case bool:
		return ResolvableString(strconv.FormatBool(v))
	case error:
		return ResolvableString(v.Error())
	case fmt.Stringer:
		return ResolvableString(v.String())
	default:
		return ResolvableString(fmt.Sprintf("%v", s))
	}
}

func Stringf(format string, args ...any) ResolvableString {
	return ResolvableString(fmt.Sprintf(format, args...))
}
