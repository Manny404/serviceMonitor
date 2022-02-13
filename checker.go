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
			serviceState.States = make([]State, 15)
			serviceState.Service = service
			serviceState.Priority = group.Priority
			if service.Priority != 0 {
				serviceState.Priority = service.Priority
			}

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

		time.Sleep(time.Duration((waitTime*1000)+rand.Intn(1000)) * time.Millisecond)

		a.check(serviceState)
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
	customTransport := http.DefaultTransport.(*http.Transport) // make shallow copy
	//customTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	client := http.Client{
		Timeout:   10 * time.Second,
		Transport: customTransport,
	}

	if serviceState.Service.Methode == "POST" {
		postBody, _ := json.Marshal(serviceState.Service.Postparam)
		responseBody := bytes.NewBuffer(postBody)
		resp, err = client.Post(generateServiceURL(serviceState.Service), "application/json", responseBody)
	} else {
		resp, err = client.Get(generateServiceURL(serviceState.Service))
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

	if !state.Ok && !a.MaintenanceMode { //&& !serviceState.States[1].Ok && serviceState.States[2].Ok {
		a.sendEmail(state, serviceState, countErrors(serviceState))
	}

	serviceState.States = prependState(serviceState.States, state)
}

// return number of errors. -1 if all states are errors
func countErrors(serviceState *ServiceState) int {
	count := 0
	for i := 1; i < len(serviceState.States); i++ {
		if serviceState.States[i].Ok {
			return count + 1
		}
		count++
	}

	return -1
}

func generateServiceURL(service Service) string {

	if service.HttpPass == "" || service.HttpUser == "" {
		return service.URL
	}

	parts := strings.Split(service.URL, "//")

	if len(parts) != 2 {
		return service.URL
	}

	return parts[0] + "//" + service.HttpUser + ":" + service.HttpPass + "@" + parts[1]
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
			err = fmt.Errorf("expiry warning: %v\n issuer: %s", resp.TLS.PeerCertificates[0].Issuer, expiry.Format(time.RFC850))
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
	copy(x[1:14], x)
	x[0] = y
	return x
}

func (a *App) sendEmail(state State, serviceState *ServiceState, errorCount int) {

	if serviceState.Service.PreventNotify {
		return
	}

	if !a.Conf.SMTPActive {
		return
	}

	for _, reportGroup := range a.Conf.ReportGroups {

		if errorCount != reportGroup.NeededErrors {
			continue
		}

		if reportGroup.MinPriority > serviceState.Priority {
			continue
		}

		// Here we do it all: connect to our server, set up a message and send it
		to := reportGroup.Emails

		moreMessageFilteredInfo := ""

		for i := 0; i < len(to); i++ {

			var send = false
			send, lastMessageBeforeFilter := a.filterNotificationReceiver(to[i])
			if !send {
				to = append(to[:i], to[i+1:]...)
				i--
			}

			if lastMessageBeforeFilter {
				moreMessageFilteredInfo = " More messages may be filtered!"
			}
		}

		msg := []byte("To: " + a.Conf.SenderEmail + " \r\n" +
			"Subject: Service " + serviceState.Service.Name + " has an error \r\n" +
			"\r\n" +
			"Service " + serviceState.Service.Name + " has an error. Statuscode: " + strconv.Itoa(state.HTTPCode) + moreMessageFilteredInfo)

		if a.Conf.SMTPUser == "" {
			err := smtp.SendMail(a.Conf.SMTPURL, nil, a.Conf.SenderEmail, to, msg)
			if err != nil {
				log.Print("Email err:")
				log.Println(err)
			}
		} else {
			//auth := smtp.PlainAuth("", a.Conf.SMTPUser, a.Conf.SMTPPass, strings.Split(a.Conf.SMTPURL, ":")[0])
			err := smtp.SendMail(a.Conf.SMTPURL, LoginAuth(a.Conf.SMTPUser, a.Conf.SMTPPass), a.Conf.SenderEmail, to, msg)
			if err != nil {
				log.Print("Email err:")
				log.Println(err)
			}
		}
	}
}

/*
	text
*/
func (a *App) filterNotificationReceiver(email string) (bool, bool) {

	a.notificationLock.Lock()

	notiLog, err := a.NotificationLog[email]

	if err {

		notiLog = &Notification{
			created: time.Now().Unix(),
		}

		a.NotificationLog[email] = notiLog
	}

	if notiLog.created > time.Now().Unix()+(60*30) {
		notiLog.created = time.Now().Unix()
		notiLog.count = 0
	}

	notiLog.count++

	a.notificationLock.Unlock()

	// last send message
	if notiLog.count == 5 {
		return true, true
	}

	// filtered out
	if notiLog.count > 5 {
		return false, false
	}

	// nomral send
	return true, false
}
