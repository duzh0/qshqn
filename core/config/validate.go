package config

import (
	"fmt"
	"reflect"
	"strings"
)

func ValidateStruct(s any) error {
	var errs []string
	validate(reflect.ValueOf(s), "", &errs)

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

func validate(v reflect.Value, parentPath string, errs *[]string) {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		val := v.Field(i)

		jsonTag := field.Tag.Get("json")
		fieldName, _, _ := strings.Cut(jsonTag, ",")
		if fieldName == "" {
			fieldName = field.Name
		}

		currentPath := fieldName
		if parentPath != "" {
			currentPath = parentPath + "." + fieldName
		}

		if field.Tag.Get("req") == "true" {
			if isZero(val) {
				*errs = append(*errs, fmt.Sprintf("[%s] is missing or empty", currentPath))
			}
		}

		if val.Kind() == reflect.Struct {
			validate(val, currentPath, errs)
		}
	}
}

func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return len(strings.TrimSpace(v.String())) == 0
	case reflect.Slice, reflect.Map:
		return v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	default:
		return v.IsZero()
	}
}
