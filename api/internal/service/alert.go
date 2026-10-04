package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EvaluatePriceDrop reports whether newPrice is at least thresholdPct percent below lastPrice.
// A threshold of 0 means "any drop".
func EvaluatePriceDrop(lastPrice, newPrice, thresholdPct float64) bool {
	if lastPrice <= 0 || newPrice <= 0 || newPrice >= lastPrice {
		return false
	}
	dropPct := (lastPrice - newPrice) / lastPrice * 100
	return dropPct >= thresholdPct
}

var lineHTTPClient = &http.Client{Timeout: 10 * time.Second}

const linePushURL = "https://api.line.me/v2/bot/message/push"

// SendLinePush sends a text message to a single LINE user via the Messaging API.
// (LINE Notify, used previously, was discontinued on 2025-03-31.)
func SendLinePush(ctx context.Context, channelToken, toUserID, message string) error {
	if channelToken == "" || toUserID == "" {
		return nil
	}
	if len([]rune(message)) > 4900 {
		message = string([]rune(message)[:4900]) + "…"
	}
	body, err := json.Marshal(map[string]any{
		"to":       toUserID,
		"messages": []map[string]string{{"type": "text", "text": message}},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, linePushURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+channelToken)

	resp, err := lineHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("LINE push failed: %s: %s", resp.Status, snippet)
	}
	return nil
}
