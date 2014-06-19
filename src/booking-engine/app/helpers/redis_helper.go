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

var c redis.Conn
var conerror error
func initRedis(){
	c, conerror = redis.Dial("tcp", ":6379")
}
func LoadSeatsIntoRedis(seatname string,sessionid string, status string ) bool{
	if c==nil {
		initRedis()
	}
	if (conerror != nil) {
		fmt.Println("error:%s", conerror)
		return false
	}
	seatkey := sessionid + "-" + seatname
	_, err := c.Do("SET", seatkey, status)
	if err!=nil {
		return false
	}
	redistatus, _ := redis.String(c.Do("GET", seatkey))
	fmt.Println(seatkey + "--" + redistatus)
	return true
}

func BlockSeat(seatkey string) bool{
	if c==nil {
		initRedis()
	}
	if ok := err==nil; ok {
		c.Do("WATCH", seatkey)
		status, _ := redis.String(c.Do("GET", seatkey))
		if status == FREE {
			c.Do("MULTI")
			c.Do("SET", seatkey, BLOCKED)
			_, err := c.Do("EXEC")
			if err==nil {
				return true
			}
		}
	}
	return false
}


func ConfirmSeat(seatkey string) bool{
	if c==nil {
		initRedis()
	}
	if ok := err==nil; ok {
		c.Do("WATCH", seatkey)
		status, _ := redis.String(c.Do("GET", seatkey))
		if status == BLOCKED {
			c.Do("MULTI")
			c.Do("SET", seatkey, CONFIRMED)
			_, err := c.Do("EXEC")
			if err==nil {
				return true
			}
		}
	}
	return false
}


