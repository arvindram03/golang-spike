package controllers

import (
	"booking-engine/app/models"
	"github.com/revel/revel"
	"log"
	"reflect"
)

type SessionController struct {
	*revel.Controller
}

func (sessionController SessionController) ScreenAvailability(sessionId int) revel.Result {
	session := &models.Session{Id: sessionId}
	seats := session.Availability()
	log.Println(reflect.TypeOf(seats))
	log.Println(seats.Front())

	return sessionController.RenderJson(seats)
}
