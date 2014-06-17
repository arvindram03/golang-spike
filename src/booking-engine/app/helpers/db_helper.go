package helpers

import (
	"github.com/coopernurse/gorp"
	"database/sql"
	_ "github.com/lib/pq"
	"fmt"
)
var dbcon *sql.DB
var err error
func initDb() {
	dbcon, err = sql.Open("postgres", "user=arvindr dbname=booking-engine sslmode=disable")
	dbcon.SetMaxOpenConns(10)
	dbcon.SetMaxIdleConns(10)

}


func GetDbMap() *gorp.DbMap {
	if dbcon==nil {
		initDb()
	}
	if ok := err==nil; ok {
		dbmap := &gorp.DbMap{Db: dbcon, Dialect: gorp.PostgresDialect{}}
		fmt.Println("dbcon",dbcon)
		fmt.Println("dbMap",dbmap)
		return dbmap;
	}
	return nil
}
