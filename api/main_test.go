package main

import (
	"testing"
	"omni-tracker-api/internal/service"
)

func TestEvaluatePriceDrop(t *testing.T) {
	tests := []struct {
		name      string
		lastPrice float64
		newPrice  float64
		expected  bool
	}{
		{"Price dropped", 100.0, 90.0, true},
		{"Price increased", 100.0, 110.0, false},
		{"Price remained same", 100.0, 100.0, false},
		{"First time scrape (0 last price)", 0.0, 90.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.EvaluatePriceDrop(tt.lastPrice, tt.newPrice, 5.0)
			if result != tt.expected {
				t.Errorf("EvaluatePriceDrop(%v, %v, 5.0) = %v; want %v", tt.lastPrice, tt.newPrice, result, tt.expected)
			}
		})
	}
}
