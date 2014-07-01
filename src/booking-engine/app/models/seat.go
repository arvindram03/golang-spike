package models

import (
	"fmt"
	"log"
	"booking-engine/app/helpers"
	"strconv"
)

type Seat struct {
	Id int
	Name string
	Status string
	SessionId int
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

	err := dbmap.SelectOne(seat,"select * from seats where name = :name and sessionid = :session_id", map[string]string {
		"name": seat.Name,
		"session_id": strconv.Itoa(seat.SessionId),
	})
	if ok := err==nil && seat.Status != helpers.CONFIRMED; ok {
		seat.Status = helpers.CONFIRMED
		_,err := dbmap.Update(seat)
		if err==nil {
			helpers.ConfirmSeat(strconv.Itoa(seat.SessionId) + seat.Name)
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

	seats := GetAllSeats()
	for _,seat :=range seats {
		helpers.LoadSeatsIntoRedis(seat.Name,strconv.Itoa(seat.SessionId),seat.Status)
	}
	return true
}




func checkErr(err error, msg string) {
	if err != nil {
		log.Println(msg, err)
	}
}
