package controllers

import (
	"github.com/revel/revel"
	"booking-engine/app/models")

type SessionController struct {
	*revel.Controller
}

func (sessionController SessionController) ScreenAvailability(sessionId int) revel.Result {
	session := &models.Session{Id: sessionId}
	seats := session.Availability()

	return sessionController.RenderJson(seats)
}
