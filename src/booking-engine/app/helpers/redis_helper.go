package helpers

import (

	"github.com/garyburd/redigo/redis"
	"fmt"
)

const (
	FREE = "free"
	BLOCKED = "blocked"
	CONFIRMED = "confirmed"
)

func LoadSeatsIntoRedis(seats map[string]string) {
	c, err := redis.Dial("tcp", ":6379")
	if (err != nil) {
		fmt.Println("error:%s", err)
		return
	}

	for seatname, status := range seats {
		c.Do("SET", seatname, status)
		fmt.Println(redis.String(c.Do("GET", seatname)))
	}

}

func BlockSeat(seatname string) bool{
	c, err := redis.Dial("tcp", ":6379")
	if ok := err==nil; ok {
		c.Do("WATCH", seatname)
		status, _ := redis.String(c.Do("GET", seatname))
		if status == FREE {
			c.Do("MULTI")
			c.Do("SET", seatname, BLOCKED)
			_, err := c.Do("EXEC")
			if err==nil {
				return true
			}
		}
	}
	return false
}


func ConfirmSeat(seatname string) bool{
	c, err := redis.Dial("tcp", ":6379")
	if ok := err==nil; ok {
		c.Do("WATCH", seatname)
		status, _ := redis.String(c.Do("GET", seatname))
		if status == BLOCKED {
			c.Do("MULTI")
			c.Do("SET", seatname, CONFIRMED)
			_, err := c.Do("EXEC")
			if err==nil {
				return true
			}
		}
	}
	return false
}


