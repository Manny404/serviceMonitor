// main.go

package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {

	fmt.Println("Starting ServiceMonitor")
	fmt.Println("Reading Config")

	file, _ := os.Open("conf.json")
	defer file.Close()
	decoder := json.NewDecoder(file)
	configuration := Configuration{}
	err := decoder.Decode(&configuration)
	if err != nil {
		fmt.Println("error Read Config:", err)
		fmt.Println("Maybe conf.json not found")
	} else {

		a := App{
			NotificationLog: make(map[string]*Notification),
		}
		a.Conf = &configuration

		a.Initialize()
		go a.InitializeChecker()

		go sayRunning()

		a.Run(configuration.Port)

	}
}

func sayRunning() {

	fmt.Println("ServiceMonitor running :)")
}
