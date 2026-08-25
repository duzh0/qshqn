package db

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/jmoiron/sqlx"

	"qshqn/core/qsh"
)

func mustGenerateSaveQuery(table string, entity any) string {
	r, err := generateSaveQuery(table, entity)
	if err != nil {
		panic(fmt.Errorf("error generating save query for entity[%T]: %w", entity, err))
	}
	return r
}

func generateSaveQuery(table string, entity any) (string, error) {
	t := reflect.TypeOf(entity)
	if t.Kind() != reflect.Struct {
		return "", fmt.Errorf("non-struct entity of type [%T] provided", entity)
	}

	var cols []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("db")

		if tag == "" || tag == "-" {
			continue
		}

		colName := strings.Split(tag, ",")[0]
		cols = append(cols, colName)
	}

	if len(cols) == 0 {
		return "", fmt.Errorf("no fields found")
	}

	columnList := strings.Join(cols, ", ")
	placeholderList := ":" + strings.Join(cols, ", :")

	return fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) VALUES (%s)", table, columnList, placeholderList), nil
}

func createDb(driver string, path string, entities map[string]any) error {
	conn, err := sqlx.Open(driver, path)
	if err != nil {
		return fmt.Errorf("create open error: %w", err)
	}
	defer conn.Close()

	for tableName, entity := range entities {
		query := generateCreateTableSQL(tableName, entity)

		qsh.Debugf("creating table [%s] with query: [%s]", tableName, query)

		if _, err := conn.Exec(query); err != nil {
			return fmt.Errorf("error creating table [%s]: %w", tableName, err)
		}
	}

	return nil
}

func generateCreateTableSQL(tableName string, entity any) string {
	t := reflect.TypeOf(entity)
	var cols []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}

		parts := strings.Split(tag, ",")
		colName := parts[0]

		isPK := false
		isNotNull := false
		isUnique := false
		defaultVal := ""

		for _, p := range parts[1:] {
			switch {
			case p == "pk":
				isPK = true
			case p == "nn":
				isNotNull = true
			case p == "uq":
				isUnique = true
			case strings.HasPrefix(p, "def="):
				defaultVal = strings.TrimPrefix(p, "def=")
			}
		}

		sqlType := "TEXT"
		switch field.Type.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32,
			reflect.Bool:
			sqlType = "INTEGER"
		case reflect.Float32, reflect.Float64:
			sqlType = "REAL"
		}

		def := fmt.Sprintf("%s %s", colName, sqlType)

		if isPK {
			def += " PRIMARY KEY NOT NULL"
		} else {
			if isNotNull {
				def += " NOT NULL"
			}
			if isUnique {
				def += " UNIQUE"
			}
			if defaultVal != "" {
				def += fmt.Sprintf(" DEFAULT %s", defaultVal)
			}
		}

		cols = append(cols, def)
	}

	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s);", tableName, strings.Join(cols, ", "))
}

func validateSchema(conn *sqlx.DB, schema map[string]any) error {
	var errors []string
	for tableName, entity := range schema {
		if err := verifyTable(conn, tableName, entity); err != nil {
			errors = append(errors, fmt.Errorf("error verifying table [%s]: %w", tableName, err).Error())
		}
	}

	if len(errors) == 0 {
		return nil
	}

	return fmt.Errorf("error validating db schema:\n%s", strings.Join(errors, "\n  "))
}

func verifyTable(conn *sqlx.DB, tableName string, entity any) error {
	rows, err := conn.Query(fmt.Sprintf("SELECT * FROM %s WHERE 1=0", tableName))
	if err != nil {
		return fmt.Errorf("table [%s] missing: %w", tableName, err)
	}
	defer rows.Close()

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return fmt.Errorf("error getting column types for [%s]: %w", tableName, err)
	}

	dbMap := make(map[string]reflect.Type)
	for _, ct := range colTypes {
		dbMap[strings.ToLower(ct.Name())] = ct.ScanType()
	}

	t := reflect.TypeOf(entity)
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("entity for table [%s] must be a struct, got %T", tableName, entity)
	}

	var errors []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}

		colName := strings.Split(tag, ",")[0]
		dbType, exists := dbMap[strings.ToLower(colName)]

		if !exists {
			errors = append(errors, fmt.Sprintf("field [%s] missing in db", colName))
			continue
		}

		if !isCompatible(field.Type, dbType) {
			errors = append(errors, fmt.Sprintf("field [%s] type mismatch: struct[%s], db[%s]", colName, field.Type, dbType))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}

	return nil
}

func isCompatible(goType, dbType reflect.Type) bool {
	if goType.ConvertibleTo(dbType) || dbType.ConvertibleTo(goType) {
		return true
	}

	if goType.Kind() == reflect.Bool && dbType.ConvertibleTo(reflect.TypeFor[int]()) {
		return true
	}

	if dbType.Kind() == reflect.Struct {
		for i := 0; i < dbType.NumField(); i++ {
			if isCompatible(goType, dbType.Field(i).Type) {
				return true
			}
		}
	}

	return false
}

func escapeMatchString(raw string) string {
	var result strings.Builder
	result.WriteString("%")

	safe := strings.ReplaceAll(raw, STRING_MATCH_ESCAPE_CHAR, STRING_MATCH_ESCAPE_CHAR+STRING_MATCH_ESCAPE_CHAR)
	safe = strings.ReplaceAll(safe, "%", STRING_MATCH_ESCAPE_CHAR+"%")
	safe = strings.ReplaceAll(safe, "_", STRING_MATCH_ESCAPE_CHAR+"_")

	result.WriteString(safe)
	result.WriteString("%")
	return result.String()
}
