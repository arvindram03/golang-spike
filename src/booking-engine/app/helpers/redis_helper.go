package helpers

import (
	"github.com/youtube/vitess/go/pools"
	"github.com/garyburd/redigo/redis"
	"fmt"
	"time"
	"log"
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

var pool *pools.ResourcePool

// ResourceConn adapts a Redigo connection to a Vitess Resource.
type ResourceConn struct {
	redis.Conn
}

func (r ResourceConn) Close() {
	r.Conn.Close()
}

func InitRedisPool() {
	pool = pools.NewResourcePool(func() (pools.Resource, error) {
			c, err := redis.Dial("tcp", ":6379")
			return ResourceConn{c}, err
		}, 1, 2, time.Minute)
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
	if pool==nil {
		log.Println("Initializing pool.")
		InitRedisPool()
	}
	if ok := err==nil; ok {
		r, _ := pool.Get()
		c := r.(ResourceConn)
		defer pool.Put(r)

		c.Do("WATCH", seatkey)
		status, _ := redis.String(c.Do("GET", seatkey))
		log.Printf("Seat %s, Status: %s", seatkey, status)
		if status == FREE {
			c.Do("MULTI")
			c.Do("SET", seatkey, BLOCKED)
			_, err := c.Do("EXEC")
			if err==nil {
				return true
			}
		}
	} else {
		log.Fatalln(err)
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


