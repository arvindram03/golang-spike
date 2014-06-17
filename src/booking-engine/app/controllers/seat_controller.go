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
	insertedAll :=false
	for i :=0;i<1000;i++ {
		seatName := "A"+strconv.Itoa(i)
		seat1 := &models.Seat{0, seatName, "free"}
		go func() {
			if !seat1.Create() {
				insertedAll = true
			}
		}()

	}
	return seat.RenderHtml("ok");
}

func (seatController SeatController) Load() revel.Result {
	status := "ko"
	if models.LoadIntoRedis() {
		status = "ok"
	}
	return seatController.RenderHtml(status);
}

func (seat SeatController) Block(seatName string) revel.Result {
	seat1 := &models.Seat{0,seatName,""}
	status := "ko"
	if seat1.Block() {
		status = "ok"
	}
	return seat.RenderHtml(status);
}

func (seat SeatController) Confirm(seatName string) revel.Result {
	seat1 := &models.Seat{0,seatName,""}

	status := "ko"
	if seat1.Confirm() {
		status = "ok"
	}
	return seat.RenderHtml(status);
}

