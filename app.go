// app.go

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

type App struct {
	Router            *mux.Router
	Conf              *Configuration
	ServiceStateGroup []*ServiceStateGroup
	MaintenanceMode   bool
	MaintenanceSetAt  int64
	NotificationLog   map[string]*Notification
	notificationLock  sync.Mutex
	StateLog          []StateLogEntry
	stateLogMutex     sync.Mutex
	Cache             int32
}

type StateLogEntry struct {
	Name     string
	Ok       bool
	HTTPCode int
	Response string
	Time     string
}

type Notification struct {
	count   int
	created int64
}

type Configuration struct {
	CheckTime    int
	MaxCheckTime int
	Port         string
	SMTPActive   bool
	ReportGroups []ReportGroup
	SMTPURL      string
	SMTPUser     string
	SMTPPass     string
	SenderEmail  string

	ServiceGroup []ServiceGroup

	SevenIo SMSInfo
}

type SMSInfo struct {
	URL      string
	Absender string
	APIKey   string
}

type ServiceGroup struct {
	Services  []Service
	Priority  int
	Name      string
	SortValue int
}

type Service struct {
	Active        bool
	PreventNotify bool
	Priority      int
	Timeout       int
	Name          string
	URL           string
	KnownBroken   bool
	Methode       string
	HttpUser      string
	HttpPass      string
	Postparam     map[string]string
}

type ServiceStateGroup struct {
	Services  []*ServiceState
	Name      string
	SortValue int
}

type ServiceState struct {
	Id           int
	Service      Service
	States       []State
	ErrorCount   int
	Priority     int
	MarkedBroken bool
	LastOk       time.Time
}

type State struct {
	Ok       bool
	HTTPCode int
	Response string
	time     time.Time
}

type ResultGroup struct {
	Services  []ResultState
	Name      string
	SortValue int
}

type Result struct {
	Groups          []ResultGroup
	StateLog        []StateLogEntry
	MaintenanceMode bool
	Cache           int32
}

type ResultState struct {
	Id         int
	Service    string
	State      ReturnState
	Name       string
	ErrorCount int
	HTTPCode   int
	Response   string
	LastOk     time.Time
	//Time       time.Time
}

type ReturnState string

const (
	OK    ReturnState = "OK"
	WARN  ReturnState = "WARN"
	ERROR ReturnState = "ERROR"
)

type ReportGroup struct {
	GroupName    string
	Emails       []string
	NeededErrors int
	MinPriority  int
}

func (a *App) Initialize() {

	go a.maintenanceReset()

	a.Router = mux.NewRouter()
	a.initializeRoutes()
}

func (a *App) initializeRoutes() {

	a.Router.HandleFunc("/api/maintenance", a.maintenance).Methods("POST")
	states := http.HandlerFunc(a.states)
	a.Router.Handle("/api/states", Gzip(states)).Queries("cache", "{cache}").Methods("GET")
	a.Router.HandleFunc("/api/markBroken", a.markBroken).Methods("POST")

	a.Router.PathPrefix("/static").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))
	a.Router.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))
}

func (a *App) Run(addr string) {
	fmt.Println("Port: " + addr)
	log.Fatal(http.ListenAndServe("127.0.0.1:"+addr, a.Router))
}

func (a *App) states(w http.ResponseWriter, r *http.Request) {

	params := mux.Vars(r)

	result := Result{}

	//fmt.Println(params["lastStateFrom"])
	//fmt.Println(a.StateLog[0].Time)
	inCache, _ := strconv.ParseInt(params["cache"], 10, 32)

	if int32(inCache) == a.Cache {

		respondWithJSON(w, 201, "OK")
		return
	}

	result.StateLog = a.StateLog
	result.Groups = make([]ResultGroup, 0)
	result.MaintenanceMode = a.MaintenanceMode
	result.Cache = a.Cache

	for _, serviceStateGroup := range a.ServiceStateGroup {

		resultGroup := ResultGroup{}
		resultGroup.Name = serviceStateGroup.Name
		resultGroup.SortValue = serviceStateGroup.SortValue
		resultGroup.Services = make([]ResultState, 0)

		for _, serviceState := range serviceStateGroup.Services {

			if serviceState == nil || !serviceState.Service.Active {
				continue
			}

			state := serviceState.States[0]

			result := ResultState{}
			result.Id = serviceState.Id
			result.Name = serviceState.Service.Name
			result.Service = serviceState.Service.URL

			if state.Ok {
				result.State = OK
				serviceState.MarkedBroken = false
			} else if serviceState.Service.KnownBroken || serviceState.MarkedBroken {
				result.State = WARN
			} else {
				result.State = ERROR
			}

			result.HTTPCode = state.HTTPCode
			result.ErrorCount = serviceState.ErrorCount
			if result.State == ERROR {
				result.Response = limitBody(state.Response)
			}
			//result.Time = state.time
			result.LastOk = serviceState.LastOk

			resultGroup.Services = append(resultGroup.Services, result)
		}
		result.Groups = append(result.Groups, resultGroup)
	}

	respondWithJSON(w, 200, result)

}

func limitBody(input string) string {

	cap := len(input) - 1

	if cap < 1 {
		return ""
	}

	if cap < 500 {
		return input
	}

	return input[:cap]
}

func findLastOk(states []State) time.Time {

	for _, state := range states {

		if state.Ok {

			return state.time
		}
	}

	return time.Time{}
}

func (a *App) markBroken(w http.ResponseWriter, r *http.Request) {

	idS, ok := r.URL.Query()["id"]
	if !ok {
		respondWithJSON(w, 400, map[string]string{"result": "MissingId"})
	}

	id, err := strconv.Atoi(idS[0])
	if err != nil || id < 0 {
		respondWithJSON(w, 400, map[string]string{"result": "id ist not a number"})
	}

	for _, group := range a.ServiceStateGroup {

		for _, service := range group.Services {

			if service != nil && service.Id == id {

				service.MarkedBroken = !service.MarkedBroken

				logEntry := StateLogEntry{}
				logEntry.Name = service.Service.Name
				timeParts := strings.Split(time.Now().String(), " ")
				logEntry.Time = timeParts[0] + "T" + timeParts[1]
				logEntry.Ok = false
				logEntry.HTTPCode = 0
				logEntry.Response = "Mark as broken"
				a.StateLog = prepend(a.StateLog, logEntry)

			}
		}
	}

	a.Cache = rand.Int31()

	respondWithJSON(w, 200, map[string]string{"result": "Ok"})
}

func (a *App) maintenance(w http.ResponseWriter, r *http.Request) {

	a.MaintenanceSetAt = time.Now().Unix()
	a.MaintenanceMode = !a.MaintenanceMode
	a.Cache = rand.Int31()
	respondWithJSON(w, 200, map[string]string{"result": "Ok"})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

func (a *App) maintenanceReset() {

	for {

		<-time.After(time.Duration(60) * time.Second)

		if a.MaintenanceMode && a.MaintenanceSetAt+(60*60) < time.Now().Unix() {
			a.MaintenanceMode = false
			a.Cache = rand.Int31()
		}
	}
}
