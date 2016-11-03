# Influx-Bench

This is a simple tool for creating benchmarks for InfluxDB.

Instead of writing code for each benchmark, benchmarks can be expressed
as part of a TOML configuration file that will describe the shape of the
data and the types of queries to run against it.

Benchmark results are output in the same format as `go test -bench=.`
and can be compared with
[benchcmp](https://github.com/golang/tools/tree/master/cmd/benchcmp).

## Configuration File

The configuration file is required to run `influx-bench`. There are no
default options. There is also currently nothing to output a sample
configuration file. The configuration file has a simple format. Each
benchmark is separated into it's own `[[benchmark]]` section as follows.

```toml
# first benchmark
[[benchmark]]
  # name of the benchmark
  name = "default_1k"
  # can be write or query
  type = "write"
  # different benchmark types of different strategies
  # the available strategies differ based on the type of benchmark
  strategy = "sparse"

# a second benchmark
[[benchmark]]
  name = "default_100k"
  type = "query"
```

The name of the benchmark will be generated into a Go-friendly benchmark
name by prepending the name with `Benchmark`, capitalizing the first
letter of the type, strategy, and name, then joining them together
separated by underscores. For example, the benchmarks above become:

```
BenchmarkWrite_Sparse_Default_1k
BenchmarkQuery_Default_Default_100k
```

Additional options are available depending on what type of benchmark and
which strategy.

All benchmarks also have additional `skip` and `seed` configuration
values. `skip = true` will cause the benchmark to be skipped. `seed =
<n>` will seed the random number generator to a specific value before
running the test.

**The random number generator is always seeded with a default value of 0
for consistency.** That option is only there for you to customize the
seed, not to determine if the test will be seeded.  Tests are never
seeded to random values.

### Write Strategies

Use `type = write` for these.

All write strategies contain these extra options:

```toml
# customize the start time of the data (defaults to 2000-01-01T00:00:00Z)
start_time = "2000-01-01T00:00:00Z"
# customize the database name (default is to randomly generate a database name)
database = "stress"
# customize the shard duration of the created database (useful for lots of shards)
shard_duration = "1h"
```

#### Default

Writes the same number of points to every series sequentially with no
gaps in the data. Each point has a 1 minute difference from each other.

```toml
[[benchmark]]
  name = "1k"
  type = "write"
  strategy = "default"
  # number of points per series
  num_points = 100
  # number of series
  cardinality = 10
```

#### Sparse

Writes data sparsely throughout the database. Each series randomly
chooses a span within the group of shards to write data to and
sequentially writes to every shards within that range.

Most useful for simulating a common infrastructure pattern that has many
ephemeral instances (like docker containers) which come alive and then
disappear forever. This writes only one point for each shard per series.

```toml
[[benchmark]]
  name = "1k"
  type = "write"
  strategy = "sparse"
  # number of shards to write a single point to
  num_shards = 100
  # number of series
  cardinality = 10
```

### Query Strategies

Use `type = query` for these.

All query strategies must have a `[benchmark.provision]` section. This
section details how the data that will be queried is provisioned so we
can test queries on different generated datasets.

```toml
[[benchmark]]
  name = "count_1k"
  type = "query"
  query = "SELECT count(value) FROM cpu"

  [benchmark.provision]
    strategy = "sparse"
    num_shards = 100
    cardinality = 10
```

There is no need to put `type = "write"` inside of the
`[benchmark.provision]` section since that will always be some write
strategy.

#### Default

Runs a simple query against the database.

```toml
[[benchmark]]
  name = "count_1k"
  type = "query"
  query = "SELECT count(value) FROM cpu"

  [benchmark.provision]
    num_points = 100
    cardinality = 10
```
