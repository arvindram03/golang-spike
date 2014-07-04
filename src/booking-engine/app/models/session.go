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

func (session *Session) Availability() []Seat {
	c := helpers.GetConnection()
	defer helpers.ReturnConnection(c)

	l := make([]Seat, 0)
	sessionId := strconv.Itoa(session.Id)

	seatKeys := getSeatKeys(c, strconv.Itoa(session.Id))
	l = getSeats(c, seatKeys, sessionId)

	return l
}

func getSeatKeys(c helpers.ResourceConn, sessionId string) string {
	seatKeys, err := redis.String(c.Do("GET", "session-"+sessionId))

	if (err != nil) {
		log.Println(err)
	}

	return seatKeys
}

func getSeats(c helpers.ResourceConn, seatKeys string, sessionId string) []Seat {
	seatKeys = strings.TrimPrefix(seatKeys, "[")
	seatKeys = strings.TrimSuffix(seatKeys, "]")
	keys := strings.Split(seatKeys, " ")

	l := make([]Seat, 0)
	var intKeys []interface {}
	for _, s := range keys {
		intKeys = append(intKeys, s)
	}

	seatStatuses, err := redis.Values(c.Do("MGET", intKeys...))
	if (err != nil) {
		log.Println(err)
	}

	var statuses []string

	if err := redis.ScanSlice(seatStatuses, &statuses); err != nil {
		log.Println(err)
	}

	for i, seatStatus := range statuses {
		seatKey := strings.TrimPrefix(keys[i], sessionId+"-")
		value, err := strconv.Atoi(sessionId)
		if err != nil {
			log.Println(err)
		}

		l = append(l, Seat{Name: seatKey, Status: seatStatus, SessionId: value})
	}

	return l
}
