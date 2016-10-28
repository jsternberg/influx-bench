package main

import (
	"fmt"
	"testing"

	influxdb "github.com/influxdata/influxdb-client"
	"github.com/mitchellh/mapstructure"
)

// [benchmark.write_sparse_data]
type Config struct {
	Benchmarks map[string]BenchmarkTemplate `toml:"benchmark"`
}

type BenchmarkTemplate map[string]interface{}

func (t BenchmarkTemplate) Create() (Benchmark, error) {
	var tmpl struct {
		Type     string
		Strategy string
	}

	if err := mapstructure.Decode(t, &tmpl); err != nil {
		return nil, err
	}
	delete(t, "strategy")
	delete(t, "type")

	switch tmpl.Type {
	case "write":
		strategy := tmpl.Strategy
		if strategy == "" {
			strategy = "default"
		}

		fn := WriteStrategies[strategy]
		if fn == nil {
			return nil, fmt.Errorf("unknown write strategy: %s", strategy)
		}
		return fn(t)
	default:
		return nil, fmt.Errorf("unknown benchmark type: %s", tmpl.Type)
	}
}

type CreateBenchmarkFn func(BenchmarkTemplate) (Benchmark, error)

type Benchmark interface {
	Run(c *influxdb.Client) (testing.BenchmarkResult, error)
}
