package models

import (
	"time"
	"booking-engine/app/helpers"
	"log"
	"github.com/garyburd/redigo/redis"
	"strconv"
	"container/list"
	"strings"
)

type Session struct {
	Id       int
	Time     time.Time
	ScreenId int
}

func (session *Session) Availability() *list.List {

	c := helpers.GetConnection()

	seatKeys, err := redis.Strings(c.Do("KEYS", strconv.Itoa(session.Id)+"-*"))

	if (err != nil) {
		log.Println(err)
		return list.New()
	}

	l := list.New()

	for _, seatKey := range seatKeys {
		seatStatus, err := redis.String(c.Do("GET", seatKey))

		if (err != nil) {
			log.Println(err)
			return list.New()
		}

		seat := Seat{Name: strings.Split(seatKey, "-")[1], Status: seatStatus}
		if seat.Name == "A421" {
			log.Println(seat)
			log.Println(seat.Name)
			log.Println(seat.Status)
		}
		l.PushBack(seat)
	}
	return l
}
