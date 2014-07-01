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

	seatKeys, err := redis.Strings(c.Do("KEYS", strconv.Itoa(session.Id)+"-*"))

	if (err != nil) {
		log.Println(err)
		return []Seat{}
	}

	l := make([]Seat, 0)

	for _, seatKey := range seatKeys {
		seatStatus, err := redis.String(c.Do("GET", seatKey))

		if (err != nil) {
			log.Println(err)
			return []Seat{}
		}

		seat := Seat{Name: strings.Split(seatKey, "-")[1], Status: seatStatus}
		l = append(l, seat)
	}
	return l
}
