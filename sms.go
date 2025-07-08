package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

func (a *App) sendSevenioSMS(number string, text string) error {

	if strings.HasPrefix(number, "0") {
		number = "49" + number[1:]
	}
	number = removeSpaces(number)

	form := url.Values{}
	form.Add("to", number) // Empfängernummer im internationalen Format
	form.Add("text", text)
	form.Add("from", a.Conf.SevenIo.Absender)

	req, err := http.NewRequest(http.MethodPost, a.Conf.SevenIo.URL+"/sms", strings.NewReader(form.Encode()))
	if err != nil {
		fmt.Printf("client: could not create request: %s\n", err)
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", a.Conf.SevenIo.APIKey)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("client: error making http request: %s\n", err)
		return err
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("client: could not read response body: %s\n", err)
		return err
	}

	resString := string(resBody)

	if !strings.Contains(resString, "\"success\":true") {
		fmt.Printf("Versandfehler: %s\n", resString)
		return errors.New("Versandfehler " + resString)
	}

	return nil
}

func removeSpaces(str string) string {
	var b strings.Builder
	b.Grow(len(str))
	for _, ch := range str {
		if !unicode.IsSpace(ch) {
			b.WriteRune(ch)
		}
	}
	return b.String()
}
