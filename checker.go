package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

func (a *App) InitializeChecker() {

	a.States = make([]*ServiceState, len(a.Conf.Services))

	for i, service := range a.Conf.Services {

		if !service.Active {
			continue
		}

		serviceState := ServiceState{}
		serviceState.States = make([]State, 10)
		serviceState.Service = service

		a.States[i] = &serviceState

		go a.checkService(&serviceState)
	}

}

func (a *App) checkService(serviceState *ServiceState) {

	for {

		waitTime := a.Conf.CheckTime + serviceState.ErrorCount

		if waitTime > a.Conf.MaxCheckTime {
			waitTime = a.Conf.MaxCheckTime
		}

		select {

		case <-time.After(time.Duration(waitTime) * time.Second):

			check(serviceState)
		}
	}
}

func check(serviceState *ServiceState) {

	// fmt.Println(serviceState.Service.URL)
	serviceState.ErrorCount++
	state := State{}
	state.time = time.Now()
	state.Ok = false

	resp, err := http.Get(serviceState.Service.URL)
	if err != nil {
		log.Println(serviceState.Service.URL)
		log.Println(err)
		state.Response = err.Error()
	} else {

		state.HTTPCode = resp.StatusCode
		defer resp.Body.Close()
		//We Read the response body on the line below.
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Println(serviceState.Service.URL)
			log.Println(err)
			state.Response = err.Error()

		} else {
			//Convert the body to type string
			sb := string(body)
			state.Response = sb
			state.Ok = true

			serviceState.ErrorCount = 0
		}
	}

	if !state.Ok && serviceState.States[1].Ok {
		sendEmail(state)
	}

	serviceState.States = prependState(serviceState.States, state)
}

func prependState(x []State, y State) []State {
	//x = append(x, State{})
	copy(x[1:9], x)
	x[0] = y
	return x
}

func sendEmail(state State) {
	// auth := smtp.PlainAuth("", "piotr@mailtrap.io", "extremely_secret_pass", "smtp.mailtrap.io")

	// // Here we do it all: connect to our server, set up a message and send it
	// to := []string{"billy@microsoft.com"}
	// msg := []byte("To: billy@microsoft.com\r\n" +
	// 	"Subject: Why are you not using Mailtrap yet?\r\n" +
	// 	"\r\n" +
	// 	"Here’s the space for our great sales pitch\r\n")
	// err := smtp.SendMail("smtp.mailtrap.io:25", auth, "piotr@mailtrap.io", to, msg)
	// if err != nil {
	// 	log.Fatal(err)
	// }
}
