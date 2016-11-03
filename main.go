package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	influxdb "github.com/influxdata/influxdb-client"
	"github.com/mitchellh/mapstructure"
)

func benchName(tmpl Template) string {
	parts := strings.Split(tmpl.Name, "_")
	for i, part := range parts {
		parts[i] = strings.Title(part)
	}
	name := strings.Join(parts, "_")
	return fmt.Sprintf("Benchmark%s_%s_%s",
		strings.Title(tmpl.Type),
		strings.Title(tmpl.Strategy),
		name)
}

func main() {
	flHost := flag.String("H", "", "host of influxdb instance")
	flConfig := flag.String("config", "", "config file to load")
	flRun := flag.String("run", "", "run filter to use for tests")
	flag.Parse()

	cfg := Config{}
	if _, err := toml.DecodeFile(*flConfig, &cfg); err != nil {
		panic(err)
	}

	var runFilter *regexp.Regexp
	if *flRun != "" {
		if re, err := regexp.Compile(*flRun); err != nil {
			fmt.Println("ERR:", err)
			os.Exit(1)
		} else {
			runFilter = re
		}
	}

	maxLen := 0
	benchmarks := make([]Template, 0, len(cfg.Benchmarks))
	for _, c := range cfg.Benchmarks {
		tmpl := Template{Strategy: "default"}
		if err := mapstructure.Decode(c, &tmpl); err != nil {
			fmt.Println("ERR:", err)
			os.Exit(1)
		}
		tmpl.Name = benchName(tmpl)
		if runFilter != nil && !runFilter.MatchString(tmpl.Name) {
			continue
		} else if len(tmpl.Name) > maxLen {
			maxLen = len(tmpl.Name)
		}

		delete(c, "name")
		delete(c, "strategy")
		delete(c, "type")

		tmpl.Config = c
		benchmarks = append(benchmarks, tmpl)
	}

	c := influxdb.Client{Addr: *flHost}
	for _, tmpl := range benchmarks {
		if tmpl.Skip {
			continue
		}

		b, err := tmpl.Create()
		if err != nil {
			fmt.Println("ERR:", err)
			os.Exit(1)
		}

		// Reseed the random number generator for each test.
		// TODO(jsternberg): Add the ability to rerun tests with different
		// seeds to try and capture potential situations that we should test.
		rand.Seed(tmpl.Seed)

		result, err := b.Run(&c)
		if err != nil {
			fmt.Println("ERR:", err)
			os.Exit(1)
		}
		fmt.Printf("%-*s\t%s\n", maxLen, tmpl.Name, result)
	}
}
