package main

import (
	"math/rand"
	"time"

	influxdb "github.com/influxdata/influxdb-client"
)

type PointGenerator struct {
	Point     influxdb.Point
	C         chan influxdb.Point
	StartTime time.Time
}

func NewPointGenerator(pt influxdb.Point, ch chan influxdb.Point, start time.Time) *PointGenerator {
	return &PointGenerator{
		Point:     pt,
		C:         ch,
		StartTime: start,
	}
}

func (g *PointGenerator) GeneratePoints(pointN int, interval time.Duration) {
	now := g.StartTime
	for i := 0; i < pointN; i++ {
		g.Point.Fields = influxdb.Value(rand.Int63())
		g.Point.Time = now
		g.C <- g.Point
		now = now.Add(interval)
	}
}
