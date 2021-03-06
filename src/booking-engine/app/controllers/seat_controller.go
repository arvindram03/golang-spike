package controllers

import (
	"github.com/revel/revel"
	"booking-engine/app/models"
	"strings"
	"strconv"
	"fmt"
	"net/http"
	"log")

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

func (seatController SeatController) LoadSets() revel.Result {
	status := "ko"
	if models.LoadSetsIntoRedis() {
		status = "ok"
	}
	return seatController.RenderHtml(status);
}

func (seat SeatController) Block(seatName string) revel.Result {
	seat1 := &models.Seat{0,seatName,"",0}
	status := "ko"
	seat.Response.Status = http.StatusBadRequest

	if seat1.Block() {
		seat.Response.Status = http.StatusOK
		status = "ok"
	}

	log.Println("seat block status: ", seatName, status)
	return seat.RenderHtml(status);
}

func (seat SeatController) Confirm(seatInfo string) revel.Result {
	fmt.Println(seatInfo)
	seatdetails := strings.Split(seatInfo,"-")
	sessionId, _ := strconv.Atoi(seatdetails[0])
	seat1 := &models.Seat{0, seatdetails[1], "", sessionId}
	status := "ko"
	if seat1.Confirm() {
		status = "ok"
	}
	return seat.RenderHtml(status);
}

