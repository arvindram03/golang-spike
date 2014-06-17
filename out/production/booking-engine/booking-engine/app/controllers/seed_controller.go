package controllers

import (
	"github.com/revel/revel"
	"booking-engine/app/models"
	"booking-engine/app/helpers"
	"time"
	"strconv"
	"github.com/coopernurse/gorp"
)

type SeedController struct {
	*revel.Controller
}

func (seedController SeedController) Seed() revel.Result {
	var seat models.Seat

	dbmap := helpers.GetDbMap()
	dbmap.AddTableWithName(models.Seat{}, "seats").SetKeys(true,"Id")
	dbmap.AddTableWithName(models.Screen{}, "screens").SetKeys(true,"Id")
	dbmap.AddTableWithName(models.Session{}, "sessions").SetKeys(true,"Id")
	dbmap.AddTableWithName(models.Theatre{}, "theatres").SetKeys(true,"Id")

	dbmap.CreateTablesIfNotExists()

	for theatreindex:=0;theatreindex<3000;theatreindex++ {

//		go func() {
			theatre := models.Theatre{0}
			dbmap.Insert(&theatre)


//		}()
	}




	return seedController.RenderHtml("ok")
}

func DumpSessions(dbmap *gorp.DbMap, theatre models.Theatre) {
	for screenindex := 0 ; screenindex < 2; screenindex++ {

		screen := models.Screen{0, theatre.Id}
		dbmap.Insert(&screen)

		for i := 0 ; i < 4 ; i++ {

			session := models.Session{0, time.Now(), screen.Id}
			dbmap.Insert(&session)

			for seatindex := 0 ; seatindex < 500 ; seatindex++ {
				{
					seat = models.Seat{0, "A"+strconv.Itoa(seatindex), "free", session.Id}
					dbmap.Insert(&seat)
				}

			}
		}

	}
}

