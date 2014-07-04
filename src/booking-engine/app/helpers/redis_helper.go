package helpers

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/youtube/vitess/go/pools"
	"log"
	"time"
	"github.com/revel/revel"
)

const (
	FREE      = "free"
	BLOCKED   = "blocked"
	CONFIRMED = "confirmed"
)

var c redis.Conn
var conerror error

func initRedis() {
	result, found := revel.Config.String("redis.server.address")
	if !found {
		log.Fatalln("DB address not found in config")
	}
	c, conerror = redis.Dial("tcp", result)
	log.Println("created redis connection")
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

func ReturnConnection(c ResourceConn) {
	pool.Put(c)
}

func GetConnectionStats() (capacity, available, maxCap, waitCount int64, waitTime, idleTimeout time.Duration) {
	return pool.Stats()
}

func InitRedisPool() {
	pool = pools.NewResourcePool(func() (pools.Resource, error) {
		c, err := redis.Dial("tcp", ":6379")
		return ResourceConn{c}, err
	}, 20, 21, time.Minute)
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
		log.Println("Failed: ", err)
		return false
	}
	return true
}

func LoadSessionIntoRedis(sessionid string, seatNames []string) string {
	r, cerr := pool.Get()
	defer pool.Put(r)

	if cerr != nil {
		log.Println("Connection error: ", cerr)
	}

	c := r.(ResourceConn)

	_, err := c.Do("SET", "session-" + sessionid, seatNames)
	log.Println("session-", sessionid)
	if err != nil {
		log.Println("Failed: ", err)
	}

	log.Println("Loaded, ", sessionid)
	return sessionid
}

func BlockSeat(seatkey string) bool {

	c := GetConnection()
	defer pool.Put(c)

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

	return false
}

func ConfirmSeat(seatkey string) bool {
	c := GetConnection()
	defer pool.Put(c)

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

	return false
}
