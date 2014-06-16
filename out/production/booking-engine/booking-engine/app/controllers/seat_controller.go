package controllers

import (
	"github.com/revel/revel"
	"booking-engine/app/models"
)

type SeatController struct {
	*revel.Controller
}
//TODO bind parameters to struct
func (seat SeatController) Create(seatName string) revel.Result {

	seat1 := &models.Seat{0,seatName,"free"}
	seat1.Create()

	return seat.RenderJson(seat1);
}

func (seatController SeatController) Load(seatName string) revel.Result {
	seat := &models.Seat{0,seatName,""}
	seats :=seat.GetByName()
	helpers.LoadSeatsIntoRedis(seats)
	return seatController.RenderJson(seats);
}

func (seat SeatController) Block() revel.Result {
	return seat.Render();
}

func (seat SeatController) Confirm() revel.Result {
	return seat.Render();
}

