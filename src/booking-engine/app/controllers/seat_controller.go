package controllers

import (
	"github.com/revel/revel"
	"booking-engine/app/models"
	"strings"
	"strconv"
)

type SeatController struct {
	*revel.Controller
}

func (seatController SeatController) Load() revel.Result {
	status := "ko"
	if models.LoadIntoRedis() {
		status = "ok"
	}
	return seatController.RenderHtml(status);
}

func (seat SeatController) Block(seatName string) revel.Result {
	seat1 := &models.Seat{0,seatName,"",0}
	status := "ko"
	if seat1.Block() {
		status = "ok"
	}
	return seat.RenderHtml(status);
}

func (seat SeatController) Confirm(seatInfo string) revel.Result {

	seatdetails := strings.Split(seatInfo,"-")
	sessionId, _ := strconv.Atoi(seatdetails[0])
	seat1 := &models.Seat{0, seatdetails[1], "", sessionId}
	status := "ko"
	if seat1.Confirm() {
		status = "ok"
	}
	return seat.RenderHtml(status);
}

