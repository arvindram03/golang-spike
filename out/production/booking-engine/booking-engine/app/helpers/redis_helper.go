package helpers

import (
	"booking-engine/app/models"
	"github.com/garyburd/redigo/redis"
	"fmt"
)



func LoadSeatsIntoRedis(seats []models.Seat) {
	c, err := redis.Dial("tcp", ":6379")
	if (err != nil) {
		fmt.Println("error:%s", err)
		return
	}

	for x,seat := range seats {
		c.Do("SET",seat.Name , seat.Status)
		redis.String(c.Do("GET", seat.Name))
	}
}
