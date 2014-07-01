package helpers

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/youtube/vitess/go/pools"
	"log"
	"time"
)

const (
	FREE      = "free"
	BLOCKED   = "blocked"
	CONFIRMED = "confirmed"
)

var c redis.Conn
var conerror error

func initRedis() {
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

func GetConnection() ResourceConn {
	connection, err := pool.Get()
	if err != nil {
		log.Println(err)
	}

	return connection.(ResourceConn)
}

func InitRedisPool() {
	pool = pools.NewResourcePool(func() (pools.Resource, error) {
		c, err := redis.Dial("tcp", ":6379")
		return ResourceConn{c}, err
	}, 10, 30, time.Minute)
}

func LoadSeatsIntoRedis(seatname string, sessionid string, status string) bool {
	if c == nil {
		initRedis()
	}
	if conerror != nil {
		fmt.Println("error:%s", conerror)
		return false
	}
	seatkey := sessionid + "-" + seatname
	_, err := c.Do("SET", seatkey, status)
	if err != nil {
		return false
	}
	redistatus, _ := redis.String(c.Do("GET", seatkey))
	fmt.Println(seatkey + "--" + redistatus)
	return true
}

func BlockSeat(seatkey string) bool {
	if ok := err == nil; ok {
		r, cerr := pool.Get()
		if cerr != nil {
			log.Println("Connection error: ", cerr)
		}

		c := r.(ResourceConn)

		defer pool.Put(r)

		if exists, _ := redis.Int(c.Do("EXISTS", seatkey)); exists == 0 {
			log.Println("No seat with key: ", seatkey)
			return false
		}

		c.Do("WATCH", seatkey)
		status, serr := redis.String(c.Do("GET", seatkey))
		log.Printf("Seat %s, Status: %s", seatkey, status)
		if serr != nil {
			log.Println("Serr: ", seatkey, serr)
		}

		if status == FREE {
			c.Do("MULTI")
			c.Do("SET", seatkey, BLOCKED)
			_, err := c.Do("EXEC")
			if err == nil {
				return true
			} else {
				log.Println(err)
			}
		}
	}

	if err != nil {
		log.Println("Err", err)
	}

	return false
}

func ConfirmSeat(seatkey string) bool {
	if c == nil {
		initRedis()
	}
	if ok := err == nil; ok {
		c.Do("WATCH", seatkey)
		status, _ := redis.String(c.Do("GET", seatkey))
		if status == BLOCKED {
			c.Do("MULTI")
			c.Do("SET", seatkey, CONFIRMED)
			_, err := c.Do("EXEC")
			if err == nil {
				return true
			}
		}
	}
	return false
}
