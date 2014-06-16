package controllers

import (
	"github.com/revel/revel"
	"booking-engine/app/models"
	"strconv"
)

type SeatController struct {
	*revel.Controller
}
//TODO bind parameters to struct
func (seat SeatController) Create() revel.Result {
	for i :=0;i<1000;i++ {
		seatName := "A"+strconv.Itoa(i)
		seat1 := &models.Seat{0, seatName, "free"}
		seat1.Create()
	}
	return seat.RenderHtml("ok");
}

func (seatController SeatController) Load() revel.Result {
	models.LoadIntoRedis()
	return seatController.RenderHtml("ok");
}

func (seat SeatController) Block(seatName string) revel.Result {
	seat1 := &models.Seat{0,seatName,""}
	seat1.Block()
	return seat.RenderJson(seat1);
}

func (seat SeatController) Confirm(seatName string) revel.Result {
	seat1 := &models.Seat{0,seatName,""}
	seat1.Confirm()
	return seat.RenderJson(seat1);
}

