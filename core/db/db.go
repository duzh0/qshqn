package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3" // _ = init() but not import

	"qshqn/core/config"
	"qshqn/core/fiox"
	"qshqn/core/qsh"
)

const (
	DRIVER          = "sqlite3"
	PRIMARY_KEY     = "id"
	CONNECT_OPTIONS = "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"

	STRING_MATCH_ESCAPE_CHAR = "\\"
)

var (
	db       *sqlx.DB
	entities = map[string]any{}
	registry = make(map[reflect.Type]entityMeta)
)

type entityMeta struct {
	table       string
	loadQuery   string
	saveQuery   string
	defaultFunc func(ID int64) any
}

func register[T any](tableName string, defaultFunc func(ID int64) *T) {
	if tableName == "" {
		panic("tried to register an empty table name")
	}
	if _, ok := entities[tableName]; ok {
		panic(fmt.Sprintf("tried to register table [%s] more than once", tableName))
	}
	if reflect.TypeFor[T]().Kind() != reflect.Struct {
		panic(fmt.Sprintf("tried to register a non-struct entity at table [%s]", tableName))
	}

	var empty T
	entities[tableName] = empty

	registry[reflect.TypeFor[T]()] = entityMeta{
		table:       tableName,
		loadQuery:   "SELECT * FROM " + tableName + " WHERE " + PRIMARY_KEY + " = ?",
		saveQuery:   mustGenerateSaveQuery(tableName, empty),
		defaultFunc: func(id int64) any { return defaultFunc(id) },
	}
}

type Entity interface {
	isEntity()
}

type PredatorMsgsInit struct {
	Path       string
	ImportMode string
}

type InitOptions struct {
	DBPath       string
	PredatorMsgs PredatorMsgsInit
}

func importPredatorMsgs(opts *InitOptions) error {
	path := opts.PredatorMsgs.Path
	if path == "" {
		qsh.Debug("skipping predator msgs import (path is empty)")
		return nil
	}

	mode := strings.ToLower(opts.PredatorMsgs.ImportMode)
	qsh.Debugf("predator msgs import mode [%s]", mode)

	if mode == config.PREDATOR_MSGS_IMPORT_MODE_SKIP {
		qsh.Debugf("skipping predator msgs import (mode=[%s])", mode)
		return nil
	}

	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM "+predatorMsgsTable); err != nil {
		return fmt.Errorf("count check error: %w", err)
	}

	if mode == config.PREDATOR_MSGS_IMPORT_MODE_UNSPECIFIED {
		if count > 0 {
			qsh.Debug("skipping predator msgs import (mode not specified && db not empty)")
			return nil
		}
		mode = config.PREDATOR_MSGS_IMPORT_MODE_UPDATE
	}

	msgs, err := fiox.Load[[]string](path, fiox.NoReadCache, fiox.NoSetCache)
	if err != nil {
		return fmt.Errorf("json load error: %w", err)
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if mode == config.PREDATOR_MSGS_IMPORT_MODE_REPLACE {
		if _, err := tx.Exec("DELETE FROM " + predatorMsgsTable); err != nil {
			return fmt.Errorf("wipe table [%s] error: %w", predatorMsgsTable, err)
		}
		count = 0
	}

	stmt, err := tx.Preparex("INSERT OR IGNORE INTO " + predatorMsgsTable + " (text) VALUES (?)")
	if err != nil {
		return err
	}

	inserted := 0
	for _, text := range msgs {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}

		res, err := stmt.Exec(text)
		if err != nil {
			return err
		}

		if rowsAffected, _ := res.RowsAffected(); rowsAffected > 0 {
			inserted++
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	if inserted > 0 {
		qsh.Infof("imported [%d] predator msgs from [%s] (mode=[%s])", inserted, path, mode)
	} else {
		qsh.Info("no new predator msgs inserted")
	}

	return nil
}

func connect(opts *InitOptions) error {
	dsn := opts.DBPath + CONNECT_OPTIONS
	conn, err := sqlx.Connect(DRIVER, dsn)
	if err != nil {
		return fmt.Errorf("db[%s] connect error: %w", opts.DBPath, err)
	}

	if err = validateSchema(conn, entities); err != nil {
		conn.Close()
		return err
	}

	db = conn

	go func() {
		if err := importPredatorMsgs(opts); err != nil {
			qsh.Errorf("error importing predator messages from [%s]: %w", opts.PredatorMsgs.Path, err)
		}
	}()

	return nil
}

func Default[T Entity](ID int64) *T {
	meta := registry[reflect.TypeFor[T]()]
	return meta.defaultFunc(ID).(*T)
}

func Keys[T Entity]() ([]int64, error) {
	meta, ok := registry[reflect.TypeFor[T]()]
	if !ok {
		var t T
		return nil, fmt.Errorf("type [%T] is not registered in db", t)
	}

	var ids []int64
	query := fmt.Sprintf("SELECT %s FROM %s", PRIMARY_KEY, meta.table)
	if err := db.Select(&ids, query); err != nil {
		return nil, fmt.Errorf("error fetching keys from table [%s]: %w", meta.table, err)
	}
	return ids, nil
}

func Exists[T Entity](ID int64) (bool, error) {
	meta, ok := registry[reflect.TypeFor[T]()]
	if !ok {
		var t T
		return false, fmt.Errorf("type [%T] is not registered in db", t)
	}

	var dummy bool
	query := fmt.Sprintf("SELECT 1 FROM %s WHERE %s = ? LIMIT 1", meta.table, PRIMARY_KEY)
	err := db.Get(&dummy, query, ID)
	if err != nil {
		if err != sql.ErrNoRows {
			return false, fmt.Errorf("error checking existence of ID [%d] in table [%s]: %w", ID, meta.table, err)
		}
		return false, nil
	}
	return true, nil
}

func Load[T Entity](ID int64) (*T, error) {
	var item T
	meta, ok := registry[reflect.TypeFor[T]()]
	if !ok {
		return &item, fmt.Errorf("type [%T] is not registered in db", item)
	}

	if err := db.Get(&item, meta.loadQuery, ID); err != nil {
		return nil, err
	}
	return &item, nil
}

func LoadOrDefault[T Entity](ID int64) (*T, error) {
	entity, err := Load[T](ID)
	if err == nil {
		return entity, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("db load error: %w", err)
	}

	defaultEntity := Default[T](ID)
	if err := Save(defaultEntity); err != nil {
		return nil, fmt.Errorf("db save default error: %w", err)
	}
	return defaultEntity, nil
}

func Save[T Entity](item *T) error {
	meta := registry[reflect.TypeFor[T]()]
	if _, err := db.NamedExec(meta.saveQuery, item); err != nil {
		return fmt.Errorf("error saving to table [%s]: %w", meta.table, err)
	}
	return nil
}

func Close() error {
	if db != nil {
		qsh.Debug("closing database connection")
		return db.Close()
	}
	return nil
}

// pass "" to create a new database
//
// returns db path and error
func Init(opts *InitOptions) (string, error) {
	qsh.Debug("initializing db module")
	path := opts.DBPath
	if path != "" {
		path = filepath.Clean(path)
		if fiox.IsAccessible(path) {
			return path, connect(opts)
		}
	}

	if !qsh.IsShellStarted() {
		if path != "" {
			return "", fmt.Errorf("db at [%s] does not exist, unable to ask because shell was not started", path)
		}
		return "", fmt.Errorf("no db path, unable to ask because shell was not started")
	}

	var create bool
	var err error
	for {
		if path != "" {
			prompt := fmt.Sprintf("db at [%s] not found. create?", path)
			create, err = qsh.YesNo(prompt)
			if err != nil {
				return "", fmt.Errorf("error asking confirmation: %w", err)
			}

			if !create {
				path = ""
				continue
			}
		} else {
			dbpath, err := qsh.Ask("provide path to create/connect db")
			if err != nil {
				return "", fmt.Errorf("error asking for db path: %w", err)
			}

			dbpath = strings.TrimSpace(dbpath)
			if dbpath == "" {
				continue
			}

			dbpath = filepath.Clean(dbpath)
			exists := fiox.IsAccessible(dbpath)
			var prompt string
			if exists {
				prompt = fmt.Sprintf("db at [%s] exists. connect?", dbpath)
			} else {
				prompt = fmt.Sprintf("create db at [%s]?", dbpath)
			}

			confirm, err := qsh.YesNo(prompt)
			if err != nil {
				return "", fmt.Errorf("error confirming: %w", err)
			}
			if !confirm {
				continue
			}

			path = dbpath
			create = !exists
		}

		if create {
			if err := createDb(DRIVER, path, entities); err != nil {
				return "", err
			}
		}

		opts.DBPath = path
		return path, connect(opts)
	}
}
