package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	influxdb "github.com/influxdata/influxdb-client"
	"github.com/mitchellh/mapstructure"
)

var WriteStrategies = make(map[string]CreateBenchmarkFn)

func RegisterWriteStrategy(name string, fn CreateBenchmarkFn) {
	WriteStrategies[name] = fn
}

type WriteStrategy interface {
	SetUp(c *influxdb.Client) (string, error)
	CleanUp(c *influxdb.Client) error
	WritePoints(c *influxdb.Client) error
}

type WriteStrategyOptions struct {
	StartTime     string `mapstructure:"start_time"`
	Database      string
	ShardDuration string `mapstructure:"shard_duration"`

	f *os.File // reference to the temporary file for an autocreated database
}

func (opt *WriteStrategyOptions) SetUp(c *influxdb.Client) (string, error) {
	if opt.Database == "" {
		f, err := ioutil.TempFile(os.TempDir(), "tmp")
		if err != nil {
			return "", err
		}
		opt.f = f
		opt.Database = filepath.Base(f.Name())
	}

	cmd := fmt.Sprintf(`CREATE DATABASE "%s"`, opt.Database)
	if opt.ShardDuration != "" {
		cmd += fmt.Sprintf(" WITH SHARD DURATION %s", opt.ShardDuration)
	}

	if err := c.Execute(cmd, nil); err != nil {
		if opt.f != nil {
			opt.Database = ""
			opt.f.Close()
			os.Remove(opt.f.Name())
		}
		return "", err
	}
	time.Sleep(100 * time.Millisecond) // small sleep since sometimes the database isn't created immediately (this should be an error on the server, but it isn't)
	return opt.Database, nil
}

func (opt *WriteStrategyOptions) CleanUp(c *influxdb.Client) error {
	if opt.f != nil {
		defer os.Remove(opt.f.Name())
		defer opt.f.Close()
		defer func() { opt.Database = "" }()
	}
	return c.Execute(fmt.Sprintf(`DROP DATABASE "%s"`, opt.Database), nil)
}

func (opt WriteStrategyOptions) GetStartTime() (time.Time, error) {
	s := opt.StartTime
	if s == "" {
		s = "2000-01-01T00:00:00Z"
	}
	return time.Parse(time.RFC3339Nano, s)
}

type DefaultWriteStrategy struct {
	NumPoints            int `mapstructure:"num_points"`
	Cardinality          int
	WriteStrategyOptions `mapstructure:",squash"`
}

func NewDefaultWriteStrategy(config map[string]interface{}) (Benchmark, error) {
	b := &DefaultWriteStrategy{
		Cardinality: 1,
	}

	if err := mapstructure.Decode(config, b); err != nil {
		return nil, err
	}

	if b.Cardinality <= 0 {
		return nil, errors.New("cardinality must be positive")
	}
	if b.NumPoints <= 0 {
		return nil, errors.New("number of points must be positive")
	}
	return b, nil
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

func (b *DefaultWriteStrategy) runOnce(c *influxdb.Client, n int) (time.Duration, error) {
	if _, err := b.SetUp(c); err != nil {
		return 0, err
	}
	defer b.CleanUp(c)
	return b.writePoints(c)
}

func (b *DefaultWriteStrategy) WritePoints(c *influxdb.Client) error {
	_, err := b.writePoints(c)
	return err
}

func (b *DefaultWriteStrategy) writePoints(c *influxdb.Client) (time.Duration, error) {
	ch := make(chan influxdb.Point, b.NumPoints+b.NumPoints/2)
	hostTemplate := fmt.Sprintf("server%%0%dd", len(strconv.Itoa(b.Cardinality-1)))

	startTime, err := b.GetStartTime()
	if err != nil {
		return 0, err
	}

	var wg sync.WaitGroup
	for i := 0; i < b.Cardinality; i++ {
		wg.Add(1)
		g := NewPointGenerator(influxdb.Point{
			Name: "cpu",
			Tags: []influxdb.Tag{
				{Key: "host", Value: fmt.Sprintf(hostTemplate, i)},
			},
		}, ch, startTime)

		go func() {
			defer wg.Done()
			g.GeneratePoints(b.NumPoints, time.Minute)
		}()
	}
	go func() { wg.Wait(); close(ch) }()

	start := time.Now()
	for {
		if err := c.WriteBatch(b.Database, influxdb.WriteOptions{}, func(w influxdb.Writer) error {
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

// SparseWriteStrategy represents a strategy to write points from different
// series to a large number of shards. The data is sparse so the series doesn't
// exist in every shard and there is a pattern for how the points are
// distributed: either sequentially or random.
//
// Sequential means that the points for a single series are in sequential
// shards. This is a common pattern for an infrastructure that has many
// ephemeral instances (like docker containers) come alive and disappear, but
// has relatively few that are active at any one time.
//
// Random means that we keep the cardinality and place the points into random
// shards. This is much more common in the IoT use-case where, to preserve
// battery and network activity, IoT devices only report something when the
// value changes. There could be millions of devices active, but not a very
// high load.
type SparseWriteStrategy struct {
	NumShards            int `mapstructure:"num_shards"`
	Cardinality          int
	WriteStrategyOptions `mapstructure:",squash"`
}

func NewSparseWriteStrategy(config map[string]interface{}) (Benchmark, error) {
	b := &SparseWriteStrategy{
		Cardinality: 1,
	}

	cfg := mapstructure.DecoderConfig{
		DecodeHook: mapstructure.StringToTimeDurationHookFunc(),
		Result:     b,
	}

	dec, err := mapstructure.NewDecoder(&cfg)
	if err != nil {
		return nil, err
	} else if err := dec.Decode(config); err != nil {
		return nil, err
	}

	if b.NumShards <= 0 {
		return nil, errors.New("number of shards must be positive")
	}
	return b, nil
}

func (b *SparseWriteStrategy) Run(c *influxdb.Client) (testing.BenchmarkResult, error) {
	return testing.BenchmarkResult{}, nil
}

func (b *SparseWriteStrategy) runOnce(c *influxdb.Client, n int) (testing.BenchmarkResult, error) {
	return testing.BenchmarkResult{}, nil
}

func (b *SparseWriteStrategy) WritePoints(c *influxdb.Client) error {
	_, err := b.writePoints(c)
	return err
}

func (b *SparseWriteStrategy) writePoints(c *influxdb.Client) (time.Duration, error) {
	// Retrieve the shard duration from the retention policy.
	sgDuration, err := b.getShardGroupDuration(c)
	if err != nil {
		return 0, err
	}

	startTime, err := b.GetStartTime()
	if err != nil {
		return 0, err
	}
	ch := make(chan influxdb.Point, b.Cardinality*b.NumShards)
	hostTemplate := fmt.Sprintf("server%%0%dd", len(strconv.Itoa(b.Cardinality-1)))

	var wg sync.WaitGroup
	for i := 0; i < b.Cardinality; i++ {
		wg.Add(1)
		startN := rand.Intn(b.NumShards)
		pointsN := rand.Intn(b.NumShards)
		if pointsN+startN > b.NumShards {
			pointsN = b.NumShards - startN
		}

		g := NewPointGenerator(influxdb.Point{
			Name: "cpu",
			Tags: []influxdb.Tag{
				{Key: "host", Value: fmt.Sprintf(hostTemplate, i)},
			},
		}, ch, startTime.Add(sgDuration*time.Duration(startN)))

		go func() {
			defer wg.Done()
			g.GeneratePoints(pointsN, sgDuration)
		}()
	}
	go func() { wg.Wait(); close(ch) }()

	start := time.Now()
	for {
		if err := c.WriteBatch(b.Database, influxdb.WriteOptions{}, func(w influxdb.Writer) error {
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

func (b *SparseWriteStrategy) getShardGroupDuration(c *influxdb.Client) (time.Duration, error) {
	cur, err := c.Select(fmt.Sprintf(`SHOW RETENTION POLICIES ON "%s"`, b.Database), nil)
	if err != nil {
		return 0, err
	}
	defer cur.Close()

	var sgDuration time.Duration
	if err := influxdb.EachResult(cur, func(result influxdb.ResultSet) error {
		return influxdb.EachSeries(result, func(series influxdb.Series) error {
			return influxdb.EachRow(series, func(row influxdb.Row) error {
				defaultRP := row.ValueByName("default")
				if defaultRP == nil {
					return nil
				}

				if defaultRP, ok := defaultRP.(bool); ok && defaultRP {
					ds := row.ValueByName("shardGroupDuration")
					if ds != nil {
						if ds, ok := ds.(string); ok {
							d, err := time.ParseDuration(ds)
							if err != nil {
								return err
							}
							sgDuration = d
							return influxdb.SKIP
						}
					}
				}
				return nil
			})
		})
	}); err != nil {
		return 0, err
	}
	return sgDuration, nil
}

func init() {
	RegisterWriteStrategy("default", NewDefaultWriteStrategy)
	RegisterWriteStrategy("sparse", NewSparseWriteStrategy)
	RegisterQueryStrategy("default", NewDefaultQueryStrategy)
}
