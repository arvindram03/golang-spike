package models

import (
	"fmt"
	"log"
	"booking-engine/app/helpers"
)

type Seat struct {
	Id int
	Name string
	Status string
}

func (seat *Seat) Create() bool{
	dbmap := helpers.GetDbMap();
	dbmap.AddTableWithName(Seat{}, "seats").SetKeys(true,"Id")
	err := dbmap.CreateTablesIfNotExists()

	if ok := err == nil; ok {
		error := dbmap.Insert(seat)
		if error==nil {
			return true
		}
		fmt.Println("error",error)
	}
	fmt.Println("err",err)
	return false
}

func (seat *Seat) Block() bool{
	if helpers.BlockSeat(seat.Name) {
		seat.Status = helpers.BLOCKED
		return true
	}
	return false
}

func (seat *Seat) Confirm() bool{
	dbmap := helpers.GetDbMap()
	dbmap.AddTableWithName(Seat{}, "seats").SetKeys(true,"Id")

	err := dbmap.SelectOne(seat,"select * from seats where name = :name", map[string]string {
		"name": seat.Name,
	})
	if ok := err==nil && seat.Status != helpers.CONFIRMED; ok {
		seat.Status = helpers.CONFIRMED
		_,err := dbmap.Update(seat)
		if err==nil {
			helpers.ConfirmSeat(seat.Name)
			return true
		}
	}
	return false
}

func GetAllSeats() []Seat{
	dbmap := helpers.GetDbMap();

	var seats []Seat
	_,err :=dbmap.Select(&seats, "select * from seats")

	fmt.Println(err)
	return seats

}
func LoadIntoRedis() bool{
	seatmap := make(map[string]string)
	seats := GetAllSeats()
	for _,seat :=range seats {
		seatmap[seat.Name] = seat.Status
	}
	return helpers.LoadSeatsIntoRedis((map[string]string)(seatmap))
}




func checkErr(err error, msg string) {
	if err != nil {
		log.Fatalln(msg, err)
	}
}
