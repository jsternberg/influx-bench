package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	influxdb "github.com/influxdata/influxdb-client"
	"github.com/mitchellh/mapstructure"
)

var QueryStrategies = make(map[string]CreateBenchmarkFn)

func RegisterQueryStrategy(name string, fn CreateBenchmarkFn) {
	QueryStrategies[name] = fn
}

type DefaultQueryStrategy struct {
	ProvisionConfig map[string]interface{} `mapstructure:"provision"`
	Provision       WriteStrategy          `mapstructure:"-"`
	Query           string
}

func NewDefaultQueryStrategy(config map[string]interface{}) (Benchmark, error) {
	b := &DefaultQueryStrategy{}
	if err := mapstructure.Decode(config, b); err != nil {
		return nil, err
	}

	if b.Query == "" {
		return nil, errors.New("query must be specified")
	}

	var writeConfig struct {
		Strategy string
	}
	if err := mapstructure.Decode(b.ProvisionConfig, &writeConfig); err != nil {
		return nil, err
	}
	delete(b.ProvisionConfig, "strategy")

	if writeConfig.Strategy == "" {
		writeConfig.Strategy = "default"
	}

	fn := WriteStrategies[writeConfig.Strategy]
	if fn == nil {
		return nil, fmt.Errorf("unknown write strategy: %s", writeConfig.Strategy)
	}
	if p, err := fn(b.ProvisionConfig); err != nil {
		return nil, err
	} else {
		b.Provision, _ = p.(WriteStrategy)
	}
	return b, nil
}

func (b *DefaultQueryStrategy) Run(c *influxdb.Client) (testing.BenchmarkResult, error) {
	db, err := b.Provision.SetUp(c)
	if err != nil {
		return testing.BenchmarkResult{}, err
	}
	defer b.Provision.CleanUp(c)

	// Provision the database once.
	if err := b.Provision.WritePoints(c); err != nil {
		return testing.BenchmarkResult{}, err
	}

	minRuns := 1
	result := testing.BenchmarkResult{}
	for {
		d, err := b.runOnce(c, db)
		if err != nil {
			return testing.BenchmarkResult{}, err
		}
		result.N++
		result.T += d

		if result.T >= time.Second {
			if result.N < minRuns {
				continue
			}
			return result, nil
		} else if result.N >= minRuns {
			minRuns <<= 1
		}
	}
}

func (b *DefaultQueryStrategy) runOnce(c *influxdb.Client, db string) (time.Duration, error) {
	opt := influxdb.QueryOptions{Database: db}
	req, err := c.NewQuery("GET", b.Query, opt)
	if err != nil {
		return 0, err
	}

	start := time.Now()
	resp, err := c.Client.Do(req)
	if err != nil {
		return 0, err
	} else if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return 0, fmt.Errorf("http status %d error", resp.StatusCode)
	}
	io.Copy(ioutil.Discard, resp.Body)
	resp.Body.Close()
	return time.Now().Sub(start), nil
}
