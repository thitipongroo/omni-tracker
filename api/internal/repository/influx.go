package repository

import (
	"log"
	"strconv"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
)

var InfluxClient influxdb2.Client
var WriteAPI api.WriteAPI
var QueryAPI api.QueryAPI

func InitInflux(url, token, org, bucket string) {
	InfluxClient = influxdb2.NewClient(url, token)
	WriteAPI = InfluxClient.WriteAPI(org, bucket)
	QueryAPI = InfluxClient.QueryAPI(org)

	// The non-blocking WriteAPI reports failures only through this channel.
	// Without draining it, write errors are silently dropped.
	go func() {
		for err := range WriteAPI.Errors() {
			log.Printf("❌ InfluxDB write error: %v", err)
		}
	}()
}

// CloseInflux flushes buffered points before shutdown so they are not lost.
func CloseInflux() {
	if InfluxClient == nil {
		return
	}
	WriteAPI.Flush()
	InfluxClient.Close()
}

func uitoa(v uint) string { return strconv.FormatUint(uint64(v), 10) }

// PIDTag is the InfluxDB tag value used for a product (its primary key).
func PIDTag(productPK uint) string { return uitoa(productPK) }
