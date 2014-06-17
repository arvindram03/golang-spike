package models

import "time"

type Session struct {
	Id int
	Time time.Time
	ScreenId int
}

