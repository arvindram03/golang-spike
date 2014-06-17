package controllers

import (
	"github.com/revel/revel"
	"booking-engine/app/models"
	"booking-engine/app/helpers"
	"github.com/coopernurse/gorp"
	"time"
	"strconv"
	"fmt"
)

const (
	THEATRES = 3000
	SCREENS = 2
	SHOWS = 4
	SEATS = 5
)

type SeedController struct {
	*revel.Controller
}

func (seedController SeedController) Seed() revel.Result {

	dbmap := helpers.GetDbMap()
	dbmap.AddTableWithName(models.Seat{}, "seats").SetKeys(true,"Id")
	dbmap.AddTableWithName(models.Screen{}, "screens").SetKeys(true,"Id")
	dbmap.AddTableWithName(models.Session{}, "sessions").SetKeys(true,"Id")
	dbmap.AddTableWithName(models.Theatre{}, "theatres").SetKeys(true,"Id")

	dbmap.CreateTablesIfNotExists()

	allSeatsChannel := make(chan int, 5)

	for theatreindex:=0;theatreindex<THEATRES;theatreindex++ {
		theatre := models.Theatre{0}
		err := dbmap.Insert(&theatre)
		if err != nil {
			fmt.Println(err)
		}

		DumpScreens(dbmap, theatre, allSeatsChannel)
	}

	for seatId := range allSeatsChannel {
		fmt.Println("SeatId: " + strconv.Itoa(seatId))
	}

	return seedController.RenderHtml("ok")
}

func DumpScreens(dbmap *gorp.DbMap, theatre models.Theatre, allSeatsChannel chan int) {
	for screenindex := 0 ; screenindex < SCREENS; screenindex++ {
		screen := models.Screen{0, theatre.Id}
		err := dbmap.Insert(&screen)
		if err != nil {
			fmt.Println(err)
		}

		DumpSessions(dbmap, screen, allSeatsChannel)
	}


}


func DumpSessions(dbmap *gorp.DbMap, screen models.Screen, allSeatsChannel chan int) {
	for i := 0 ; i < SHOWS ; i++ {

		session := models.Session{0, time.Now(), screen.Id}
		err := dbmap.Insert(&session)
		if err != nil {
			fmt.Println(err)
		}


		go DumpSeats(dbmap, session, allSeatsChannel)
	}
}

func DumpSeats(dbmap *gorp.DbMap, session models.Session, allSeatsChannel chan int) {
	for seatindex := 0 ; seatindex < SEATS ; seatindex++ {
		seat := models.Seat{0, "A"+strconv.Itoa(seatindex), "free", session.Id}
		err := dbmap.Insert(&seat)
		if err != nil {
			fmt.Println(err)
		}
		allSeatsChannel <- seat.Id
	}

	fmt.Printf("Created sessions : %v\n", session.Id)
}
