package repository

import (
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
}
