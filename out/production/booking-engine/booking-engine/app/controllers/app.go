package controllers

import (
	"github.com/revel/revel"
	"time"
)

type App struct {
	*revel.Controller
}

func (c App) Index() revel.Result {
	return c.Render()
}

func (c App) Hello(myName string) revel.Result {
	return c.Render(myName, time.Now());
}


func (c App) SeatLayout(myName string) revel.Result {
	return c.Render(myName, time.Now());
}

