package main

import (
	"time"

	influxdb "github.com/influxdata/influxdb-client"
)

type PointGenerator struct {
	Name string
	Tags []influxdb.Tag
	C    chan influxdb.Point
}

func (g *PointGenerator) GeneratePoints(pointN int) {
	now, _ := time.Parse(time.RFC3339, "2010-01-01T00:00:00Z")
	for i := 0; i < pointN; i++ {
		g.C <- influxdb.NewPointWithTags(g.Name, g.Tags, influxdb.Value(float64(i)), now)
		now = now.Add(time.Minute)
	}
}
