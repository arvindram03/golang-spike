package helpers

import (
	"github.com/coopernurse/gorp"
	"database/sql"
	_ "github.com/lib/pq"
)

func InitDb() *gorp.DbMap {
	db, err := sql.Open("postgres", "user=arvindr dbname=booking-engine sslmode=disable")
	if ok :=err==nil; ok {
		dbmap := &gorp.DbMap{Db: db, Dialect: gorp.PostgresDialect{}}
		return dbmap;
	}
 	return nil
}

