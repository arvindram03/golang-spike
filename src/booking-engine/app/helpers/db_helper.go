package helpers

import (
	"github.com/coopernurse/gorp"
	"database/sql"
	_ "github.com/lib/pq"
	"fmt"
	"github.com/revel/revel"
	"log"
)
var dbcon *sql.DB
var err error

func initDb() {
	psql_user, psql_user_found := revel.Config.String("psql.user")
	psql_host, psql_host_found := revel.Config.String("psql.host")
	if !psql_user_found || !psql_host_found {
		log.Fatalln("Psql details not found")
	}

	dbcon, err = sql.Open("postgres", "user="+ psql_user+"host="+psql_host+" dbname=booking-engine sslmode=disable")
	dbcon.SetMaxOpenConns(40)
	dbcon.SetMaxIdleConns(4)
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
