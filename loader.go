package main

import (
	"fmt"
	"testing"

	influxdb "github.com/influxdata/influxdb-client"
)

// [benchmark.write_sparse_data]
type Config struct {
	Benchmarks []map[string]interface{} `toml:"benchmark"`
}

type Template struct {
	Name     string
	Type     string
	Strategy string
	Config   map[string]interface{}
}

func (t *Template) Create() (Benchmark, error) {
	switch t.Type {
	case "write":
		strategy := t.Strategy
		if strategy == "" {
			strategy = "default"
		}

		fn := WriteStrategies[strategy]
		if fn == nil {
			return nil, fmt.Errorf("unknown write strategy: %s", strategy)
		}
		return fn(t.Config)
	default:
		return nil, fmt.Errorf("unknown benchmark type: %s", t.Type)
	}
}

type CreateBenchmarkFn func(map[string]interface{}) (Benchmark, error)

type Benchmark interface {
	Run(c *influxdb.Client) (testing.BenchmarkResult, error)
}
