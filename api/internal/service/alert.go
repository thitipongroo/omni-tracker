package service

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

func EvaluatePriceDrop(lastPrice, newPrice float64) bool {
	if lastPrice > 0 && newPrice < lastPrice {
		return true
	}
	return false
}

func SendLineAlert(token, message string) {
	if token == "" || token == "your_line_notify_token_here" {
		return
	}
	apiURL := "https://notify-api.line.me/api/notify"
	data := url.Values{}
	data.Set("message", message)
	req, _ := http.NewRequest("POST", apiURL, strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 10 * time.Second}
	_, _ = client.Do(req)
}
