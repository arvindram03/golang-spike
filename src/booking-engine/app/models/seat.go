package models

import (
	"fmt"
	"log"
	"booking-engine/app/helpers"
	"strconv"
	"time"
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
	dbmap := helpers.GetDbMap()

	var seats []Seat
	dbmap.Select(&seats, "select * from seats")

	return seats

}

func GetSeats(sessionId int) []Seat{
	dbmap := helpers.GetDbMap()

	var seats []Seat
	_, err := dbmap.Select(&seats, "select * from seats where sessionid = $1", strconv.Itoa(sessionId))
	if (err != nil) {
		log.Println(err)
	}

	return seats

}

func GetAllSession() []Session{
	dbmap := helpers.GetDbMap()

	var session []Session
	_, err := dbmap.Select(&session, "select * from sessions")
	if (err != nil) {
		log.Println(err)
	}

	fmt.Println(len(session))
	return session

}
func LoadIntoRedis() bool{

	seats := GetAllSeats()

	for _,seat :=range seats {
		log.Println(seat.Name)
		helpers.LoadSeatsIntoRedis(seat.Name,strconv.Itoa(seat.SessionId),seat.Status)
		log.Println("Loaded to redis")
	}
	return true
}

func LoadSetsIntoRedis() bool{

	sessions := GetAllSession()
	chen := make(chan string, 1000)


	start := time.Now()

	go populateFromDbToRedis(sessions, chen)

	for id := range chen {
		log.Println("chen out: ", id)
	}

	log.Println("took ", time.Since(start))

	return true
}

func populateFromDbToRedis(sessions []Session, chen chan string) {
	for _, session := range sessions {
		log.Println("session: \t\t\t\t\t", session.Id)
		seatNames := GetAllSeatKeys(session.Id)
		chen <- helpers.LoadSessionIntoRedis(strconv.Itoa(session.Id), seatNames)
	}

	close(chen)
}

func GetAllSeatKeys(sessionId int) ([]string) {
	seats := GetSeats(sessionId)
	seatNames := make([]string, 0)

	for _, seat := range seats {
		seatNames = append(seatNames, strconv.Itoa(sessionId)+"-"+seat.Name)
	}

	return seatNames
}


func checkErr(err error, msg string) {
	if err != nil {
		log.Println(msg, err)
	}
}
