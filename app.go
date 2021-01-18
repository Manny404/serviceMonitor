// app.go

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

type App struct {
	Router *mux.Router
	Conf   *Configuration
	States []*ServiceState
}

type Configuration struct {
	CheckTime    int
	MaxCheckTime int
	Port         string
	SMTPActive   bool
	ReportEmails []string
	SMTPURL      string
	SMTPUser     string
	SMTPPass     string
	SenderEmail  string

	Services []Service
}

type ServiceState struct {
	Service    Service
	States     []State
	ErrorCount int
}

type State struct {
	Ok       bool
	HTTPCode int
	Response string
	time     time.Time
}

type ResultState struct {
	Service    string
	Ok         bool
	Name       string
	ErrorCount int
	HTTPCode   int
	Response   string
	LastOk     time.Time
	Time       time.Time
}

type Service struct {
	Active    bool
	Name      string
	URL       string
	Methode   string
	Postparam map[string]string
}

func (a *App) Initialize() {

	a.Router = mux.NewRouter()
	a.initializeRoutes()
}

func (a *App) initializeRoutes() {

	a.Router.PathPrefix("/static").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))

	a.Router.HandleFunc("/info", a.info).Methods("GET")
	a.Router.HandleFunc("/api/states", a.states).Methods("GET")
}

func (a *App) Run(addr string) {
	fmt.Println("Port: " + addr)
	log.Fatal(http.ListenAndServe(addr, a.Router))
}

func (a *App) states(w http.ResponseWriter, r *http.Request) {

	results := make([]ResultState, 0)

	for _, serviceState := range a.States {

		if serviceState == nil || !serviceState.Service.Active {
			continue
		}

		state := serviceState.States[0]

		result := ResultState{}
		result.Name = serviceState.Service.Name
		result.Service = serviceState.Service.URL
		result.Ok = state.Ok
		result.HTTPCode = state.HTTPCode
		result.ErrorCount = serviceState.ErrorCount
		result.Response = limitBody(state.Response)
		result.Time = state.time
		result.LastOk = findLastOk(serviceState.States)

		results = append(results, result)
	}

	respondWithJSON(w, 200, results)
}

func limitBody(input string) string {

	cap := len(input) - 1

	if cap < 1 {
		return ""
	}

	if cap > 200 {
		cap = 200
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

func (a *App) info(w http.ResponseWriter, r *http.Request) {
	value := "usersettings"
	respondWithJSON(w, 200, map[string]string{"name": value})
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}
