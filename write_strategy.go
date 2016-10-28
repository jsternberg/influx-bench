package main

import (
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	influxdb "github.com/influxdata/influxdb-client"
)

var WriteStrategies = make(map[string]CreateBenchmarkFn)

func RegisterWriteStrategy(name string, fn CreateBenchmarkFn) {
	WriteStrategies[name] = fn
}

type DefaultWriteStrategy struct{}

func NewDefaultWriteStrategy(t BenchmarkTemplate) (Benchmark, error) {
	return &DefaultWriteStrategy{}, nil
}

func (b *DefaultWriteStrategy) Run(c *influxdb.Client) (testing.BenchmarkResult, error) {
	minRuns := 1
	result := testing.BenchmarkResult{}
	for {
		d, err := b.runOnce(c, result.N+1)
		if err != nil {
			return testing.BenchmarkResult{}, err
		}
		result.N++
		result.T += d

		if result.T >= 10*time.Second {
			if result.N < minRuns {
				continue
			}
			return result, nil
		} else if result.N >= minRuns {
			minRuns <<= 1
		}
	}
}

func (b *DefaultWriteStrategy) runOnce(c *influxdb.Client, n int) (time.Duration, error) {
	db := fmt.Sprintf("tmp_%d", n)
	c.Execute(fmt.Sprintf(`CREATE DATABASE "%s"`, db), nil)
	defer c.Execute(fmt.Sprintf(`DROP DATABASE "%s"`, db), nil)

	ch := make(chan influxdb.Point, 1500)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		g := PointGenerator{
			Name: "cpu",
			Tags: []influxdb.Tag{{Key: "host", Value: fmt.Sprintf("server%02d", i)}},
			C:    ch,
		}
		go func() {
			defer wg.Done()
			g.GeneratePoints(10 * 60)
		}()
	}
	go func() { wg.Wait(); close(ch) }()

	start := time.Now()
	for {
		if err := c.WriteBatch(db, influxdb.WriteOptions{}, func(w influxdb.Writer) error {
			for i := 0; i < 1000; i++ {
				pt, ok := <-ch
				if !ok {
					return io.EOF
				} else if err := w.WritePoint(pt); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			if err != io.EOF {
				return 0, err
			}
			break
		}
	}
	return time.Now().Sub(start), nil
}

func init() {
	RegisterWriteStrategy("default", NewDefaultWriteStrategy)
}
