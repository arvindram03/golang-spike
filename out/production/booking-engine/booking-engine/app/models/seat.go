package models

import (
	"log"
	"booking-engine/app/helpers"
	"fmt"
)

type Seat struct {
	Id int
	Name string
	Status string
}

func (seat *Seat) block() bool {
	if(seat.Status != "blocked") {
		seat.Status = "blocked"
		return true;
	}
	return false;
}

func (seat *Seat) Create() {
	dbmap := helpers.InitDb();
	defer dbmap.Db.Close()

	dbmap.AddTableWithName(Seat{}, "seats").SetKeys(true,"Id")
	err := dbmap.CreateTablesIfNotExists()
	if ok := err == nil; ok {
		dbmap.Insert(seat)
	}
}

func (seat *Seat) GetByName() []Seat{
	dbmap := helpers.InitDb();
	defer dbmap.Db.Close()

	var seats []Seat
	_,err :=dbmap.Select(&seats, "select * from seats")

	fmt.Println(err)
	return seats

}


func checkErr(err error, msg string) {
	if err != nil {
		log.Fatalln(msg, err)
	}
}
