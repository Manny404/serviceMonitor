package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

func (a *App) InitializeChecker() {

	a.ServiceStateGroup = make([]*ServiceStateGroup, len(a.Conf.ServiceGroup))

	for i, group := range a.Conf.ServiceGroup {

		serviceGroup := ServiceStateGroup{}
		serviceGroup.Name = group.Name
		serviceGroup.PlayAlarm = group.PlayAlarm
		serviceGroup.SortValue = group.SortValue
		serviceGroup.Services = make([]*ServiceState, len(group.Services))
		a.ServiceStateGroup[i] = &serviceGroup

		for y, service := range group.Services {

			if !service.Active {
				continue
			}

			serviceState := ServiceState{}
			serviceState.States = make([]State, 10)
			serviceState.Service = service

			serviceGroup.Services[y] = &serviceState

			go a.checkService(&serviceState)
		}
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

		case <-time.After(time.Duration((waitTime*1000)+rand.Intn(1000)) * time.Millisecond):

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
	//customTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	client := http.Client{
		Timeout:   10 * time.Second,
		Transport: customTransport,
	}

	if serviceState.Service.Methode == "POST" {
		postBody, _ := json.Marshal(serviceState.Service.Postparam)
		responseBody := bytes.NewBuffer(postBody)
		resp, err = client.Post(serviceState.Service.URL, "application/json", responseBody)
	} else {
		resp, err = client.Get(serviceState.Service.URL)
	}

	var body string
	body, err = parseResponse(resp, err, &state, serviceState)

	if err != nil {
		state.Ok = false
		state.Response = err.Error()

	} else {
		state.Ok = true
		state.Response = body
		serviceState.ErrorCount = 0
	}

	if !state.Ok && !serviceState.States[1].Ok && serviceState.States[2].Ok {
		a.sendEmail(state, serviceState)
	}

	serviceState.States = prependState(serviceState.States, state)
}

func parseResponse(resp *http.Response, err error, state *State, serviceState *ServiceState) (string, error) {

	if err != nil {
		log.Println(err)
		return "", err
	}
	defer resp.Body.Close()

	if strings.HasPrefix(serviceState.Service.URL, "https") {
		expiry := resp.TLS.PeerCertificates[0].NotAfter
		// 4 weeks
		in4Weeks := time.Now().AddDate(0, 0, 4)

		if expiry.Before(in4Weeks) {
			err = fmt.Errorf("Expiry warning: %v\n Issuer: %s\n", resp.TLS.PeerCertificates[0].Issuer, expiry.Format(time.RFC850))
			return "", err
		}
	}

	state.HTTPCode = resp.StatusCode

	//We Read the response body on the line below.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		return "", err
	}

	if state.HTTPCode != 200 {
		return "", errors.New("Http Statuscode invalid: " + strconv.Itoa(state.HTTPCode))
	}

	//Convert the body to type string
	sb := string(body)

	return sb, nil
}

func prependState(x []State, y State) []State {
	//x = append(x, State{})
	copy(x[1:9], x)
	x[0] = y
	return x
}

func (a *App) sendEmail(state State, serviceState *ServiceState) {

	if serviceState.Service.PreventNotify {
		return
	}

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
			log.Print("Email err:")
			log.Println(err)
		}
	} else {
		auth := smtp.PlainAuth("", a.Conf.SMTPUser, a.Conf.SMTPPass, a.Conf.SMTPURL)
		err := smtp.SendMail(a.Conf.SMTPURL, auth, a.Conf.SenderEmail, to, msg)
		if err != nil {
			log.Print("Email err:")
			log.Println(err)
		}
	}
	//}

}
