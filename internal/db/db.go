package db

import (
	"database/sql"
	"math"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
)

const DSN = "_journal_mode=WAL&_busy_timeout=30000&_synchronous=NORMAL&_cache=shared"

func init() {
	sql.Register("sqlite3_glean", &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			if err := conn.RegisterFunc("exp", func(x float64) float64 { return math.Exp(x) }, true); err != nil {
				return err
			}
			if err := conn.RegisterFunc("log", func(x float64) float64 { return math.Log(x) }, true); err != nil {
				return err
			}
			pragmas := []string{
				`PRAGMA wal_autocheckpoint = 1000`,
				`PRAGMA temp_store = MEMORY`,
				`PRAGMA mmap_size = 268435456`,
			}
			for _, p := range pragmas {
				if _, err := conn.Exec(p, nil); err != nil {
					return err
				}
			}
			return nil
		},
	})
}

func NullStr(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func NullTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t, Valid: !t.IsZero()}
}

func NullInt(n int64) sql.NullInt64 {
	return sql.NullInt64{Int64: n, Valid: true}
}

func NullStrTags(tags []string) sql.NullString {
	if len(tags) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{String: strings.Join(tags, ","), Valid: true}
}

type DB struct {
	*sql.DB
}
