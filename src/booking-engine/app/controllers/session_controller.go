package controllers

import (
	"booking-engine/app/models"
	"github.com/revel/revel"
)

type SessionController struct {
	*revel.Controller
}


func (sessionController SessionController) ScreenAvailability(sessionId int) revel.Result {
	session := &models.Session{Id: sessionId}
	seats := session.Availability()

	return sessionController.RenderJson(seats)
}
