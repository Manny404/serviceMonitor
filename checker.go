package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/smtp"
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

	firstrun := true

	for {

		waitTime := a.Conf.CheckTime + serviceState.ErrorCount

		if waitTime > a.Conf.MaxCheckTime {
			waitTime = a.Conf.MaxCheckTime
		}

		if firstrun {
			firstrun = false
			waitTime = 0
		}

		select {

		case <-time.After(time.Duration(waitTime) * time.Second):

			a.check(serviceState)
		}
	}
}

func (a *App) check(serviceState *ServiceState) {

	// fmt.Println(serviceState.Service.URL)
	serviceState.ErrorCount++
	state := State{}
	state.time = time.Now()
	state.Ok = false

	var resp *http.Response
	var err error
	customTransport := &(*http.DefaultTransport.(*http.Transport)) // make shallow copy
	customTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	client := http.Client{
		Timeout: 10 * time.Second,
		Transport: customTransport,
	}

	if serviceState.Service.Methode == "POST" {
		postBody, _ := json.Marshal(serviceState.Service.Postparam)
		responseBody := bytes.NewBuffer(postBody)
		resp, err = client.Post(serviceState.Service.URL, "application/json", responseBody)
	} else {
		resp, err = client.Get(serviceState.Service.URL)
	}

	if err != nil {
		log.Println(err)
		state.Response = err.Error()
	} else {

		state.HTTPCode = resp.StatusCode
		defer resp.Body.Close()
		//We Read the response body on the line below.
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Println(err)
			state.Response = err.Error()

		} else {
			//Convert the body to type string
			sb := string(body)
			state.Response = sb
			state.Ok = true

			if state.HTTPCode != 200 {
				state.Ok = false
			}

			serviceState.ErrorCount = 0
		}
	}

	if !state.Ok && serviceState.States[1].Ok {
		a.sendEmail(state, serviceState)
	}

	serviceState.States = prependState(serviceState.States, state)
}

func prependState(x []State, y State) []State {
	//x = append(x, State{})
	copy(x[1:9], x)
	x[0] = y
	return x
}

func (a *App) sendEmail(state State, serviceState *ServiceState) {

	if !a.Conf.SMTPActive {
		return
	}

	

	//for _, _ := range a.Conf.ReportEmails {
		// Here we do it all: connect to our server, set up a message and send it
		to := a.Conf.ReportEmails
		msg := []byte("To: G111@hse.ag \r\n" +
			"Subject: Service " + serviceState.Service.URL + " has an error \r\n" +
			"\r\n" +
			"Service " + serviceState.Service.URL + " has an error. " + state.Response + " \r\n")

		if a.Conf.SMTPUser == "" {
			err := smtp.SendMail(a.Conf.SMTPURL, nil, a.Conf.SenderEmail, to, msg)
			if err != nil {
				log.Println(err)
			}
		} else {
			auth := smtp.PlainAuth("", a.Conf.SMTPUser, a.Conf.SMTPPass, a.Conf.SMTPURL)
			err := smtp.SendMail(a.Conf.SMTPURL, auth, a.Conf.SenderEmail, to, msg)
			if err != nil {
				log.Println(err)
			}
		}
	//}
	
	

}
