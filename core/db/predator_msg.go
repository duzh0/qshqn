package db

import (
	"fmt"
	"reflect"
	"strings"
)

const (
	predatorMsgsTable           = "predator_msgs"
	predatorMsgTextFieldTag     = "text"
	baseGetPredatorMsgsQuery    = "SELECT text FROM " + predatorMsgsTable + " WHERE 1=1"
	predatorMsgLenFilterQuery   = " AND length(" + predatorMsgTextFieldTag + ") <= ?"
	predatorMsgMatchClause      = predatorMsgTextFieldTag + " LIKE ? ESCAPE '" + STRING_MATCH_ESCAPE_CHAR + "'"
	predatorMsgRandomOrderQuery = " ORDER BY RANDOM() LIMIT ?"
	predatorMsgOrderQuery       = " ORDER BY id"
	predatorMsgLimitQuery       = " LIMIT ? OFFSET ?"
	addPredatorMsgQuery         = "INSERT OR IGNORE INTO " + predatorMsgsTable + " (text) VALUES (?)"
)

type PredatorMsg struct {
	ID   int64  `db:"id,pk"`
	Text string `db:"text,uq"`
}

func (PredatorMsg) isEntity() {}

func init() {
	t := reflect.TypeFor[PredatorMsg]()
	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("type of PredatorMsg is not a struct, but [%s]", t.Kind().String()))
	}
	found := false
	for field := range t.Fields() {
		if strings.Split(field.Tag.Get("db"), ",")[0] == predatorMsgTextFieldTag {
			found = true
			break
		}
	}
	if !found {
		panic(fmt.Sprintf("predator msg text field tag [%s] not found in any field of PredatorMsg struct", predatorMsgTextFieldTag))
	}
	register(predatorMsgsTable, func(ID int64) *PredatorMsg {
		return &PredatorMsg{
			ID:   ID,
			Text: "",
		}
	})
}

func buildPredatorQuery(base string, maxLen int, contains ...string) (string, []any) {
	var args []any
	query := base
	if maxLen > 0 {
		query += predatorMsgLenFilterQuery
		args = append(args, maxLen)
	}

	var terms []string
	for _, c := range contains {
		for part := range strings.SplitSeq(c, "|") {
			part = strings.TrimSpace(part)
			if part != "" {
				terms = append(terms, part)
			}
		}
	}

	if len(terms) > 0 {
		var clauses []string
		for _, term := range terms {
			clauses = append(clauses, predatorMsgMatchClause)
			args = append(args, escapeMatchString(term))
		}
		query += " AND (" + strings.Join(clauses, " OR ") + ")"
	}

	return query, args
}

func PredatorMsgs(amount, maxLen, offset int, contains ...string) ([]string, error) {
	query, args := buildPredatorQuery(baseGetPredatorMsgsQuery, maxLen, contains...)
	query += predatorMsgOrderQuery + predatorMsgLimitQuery
	args = append(args, amount, offset)
	var msgs []string
	return msgs, db.Select(&msgs, query, args...)
}

func RandPredatorMsgs(amount, maxLen int, contains ...string) ([]string, error) {
	var msgs []string
	query, args := buildPredatorQuery(baseGetPredatorMsgsQuery, maxLen, contains...)
	if amount < 1 {
		amount = 1
	}
	query += predatorMsgRandomOrderQuery
	args = append(args, amount)
	msgs = make([]string, 0, amount)
	return msgs, db.Select(&msgs, query, args...)
}

func AddPredatorMsg(text string) (bool, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return false, nil
	}
	res, err := db.Exec(addPredatorMsgQuery, text)
	if err != nil {
		return false, err
	}
	rowsAffected, err := res.RowsAffected()
	return rowsAffected > 0, nil
}
