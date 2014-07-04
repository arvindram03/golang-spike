package models

import (
	"time"
	"booking-engine/app/helpers"
	"log"
	"github.com/garyburd/redigo/redis"
	"strconv"
	"strings"
)

type Session struct {
	Id       int
	Time     time.Time
	ScreenId int
}

func getSeatKeys(c helpers.ResourceConn, sessionId string) string {
	seatKeys, err := redis.String(c.Do("GET", "session-"+sessionId))

	if (err != nil) {
		log.Println(err)
	}

	return seatKeys
}

func (session *Session) Availability() []Seat {
	c := helpers.GetConnection()
	defer helpers.ReturnConnection(c)

	l := make([]Seat, 0)
	sessionId := strconv.Itoa(session.Id)
	seatKeys := getSeatKeys(c, strconv.Itoa(session.Id))

	l = getSeats(c, seatKeys, sessionId)

	return l
}

func getSeats(c helpers.ResourceConn, seatKeys string, sessionId string) []Seat {
	seatKeys = strings.TrimPrefix(seatKeys, "[")
	seatKeys = strings.TrimSuffix(seatKeys, "]")
	log.Println(strings.Split(seatKeys, " "))
	seatKeys = strings.Replace(seatKeys, " ", " "+sessionId+"-", 0)
	seatKeys = " "+sessionId+"-" + seatKeys
	keys := strings.Split(seatKeys, " ")

//	log.Println(seatKeys)

	l := make([]Seat, 0)

	seatStatuses, err := redis.Strings(c.Do("MGET", seatKeys))
	if (err != nil) {
		log.Println(err)
	}

	for i, seatStatus := range seatStatuses {
		log.Println(seatStatus, i, keys[i])
		l = append(l, Seat{Name: keys[i], Status: seatStatus})
	}

	return l
}
