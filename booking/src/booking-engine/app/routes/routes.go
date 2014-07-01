// GENERATED CODE - DO NOT EDIT
package routes

import "github.com/revel/revel"


type tApp struct {}
var App tApp


func (_ tApp) Index(
		) string {
	args := make(map[string]string)
	
	return revel.MainRouter.Reverse("App.Index", args).Url
}

func (_ tApp) Hello(
		myName string,
		) string {
	args := make(map[string]string)
	
	revel.Unbind(args, "myName", myName)
	return revel.MainRouter.Reverse("App.Hello", args).Url
}

func (_ tApp) SeatLayout(
		myName string,
		) string {
	args := make(map[string]string)
	
	revel.Unbind(args, "myName", myName)
	return revel.MainRouter.Reverse("App.SeatLayout", args).Url
}


type tSeatController struct {}
var SeatController tSeatController


func (_ tSeatController) Load(
		) string {
	args := make(map[string]string)
	
	return revel.MainRouter.Reverse("SeatController.Load", args).Url
}

func (_ tSeatController) Block(
		seatName string,
		) string {
	args := make(map[string]string)
	
	revel.Unbind(args, "seatName", seatName)
	return revel.MainRouter.Reverse("SeatController.Block", args).Url
}

func (_ tSeatController) Confirm(
		seatInfo string,
		) string {
	args := make(map[string]string)
	
	revel.Unbind(args, "seatInfo", seatInfo)
	return revel.MainRouter.Reverse("SeatController.Confirm", args).Url
}


type tSeedController struct {}
var SeedController tSeedController


func (_ tSeedController) Seed(
		) string {
	args := make(map[string]string)
	
	return revel.MainRouter.Reverse("SeedController.Seed", args).Url
}


type tSessionController struct {}
var SessionController tSessionController


func (_ tSessionController) ScreenAvailability(
		sessionId int,
		) string {
	args := make(map[string]string)
	
	revel.Unbind(args, "sessionId", sessionId)
	return revel.MainRouter.Reverse("SessionController.ScreenAvailability", args).Url
}


type tStatic struct {}
var Static tStatic


func (_ tStatic) Serve(
		prefix string,
		filepath string,
		) string {
	args := make(map[string]string)
	
	revel.Unbind(args, "prefix", prefix)
	revel.Unbind(args, "filepath", filepath)
	return revel.MainRouter.Reverse("Static.Serve", args).Url
}

func (_ tStatic) ServeModule(
		moduleName string,
		prefix string,
		filepath string,
		) string {
	args := make(map[string]string)
	
	revel.Unbind(args, "moduleName", moduleName)
	revel.Unbind(args, "prefix", prefix)
	revel.Unbind(args, "filepath", filepath)
	return revel.MainRouter.Reverse("Static.ServeModule", args).Url
}


