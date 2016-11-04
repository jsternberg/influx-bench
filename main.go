package main

import (
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	influxdb "github.com/jsternberg/influxdb-client"
	"github.com/mitchellh/mapstructure"
	flag "github.com/spf13/pflag"
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

func realMain() int {
	flHost := flag.StringP("host", "H", "", "host of influxdb instance")
	flConfig := flag.StringP("config", "c", "", "config file to load")
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
	exitStatus := 0

	maxLen := 0
	benchmarks := make([]Template, 0, len(cfg.Benchmarks))
	for _, c := range cfg.Benchmarks {
		tmpl := Template{Strategy: "default"}
		if err := mapstructure.Decode(c, &tmpl); err != nil {
			fmt.Fprintf(os.Stderr, "ERR: %s\n", err)
			exitStatus = 1
			continue
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

	mustPing := true
	c := influxdb.Client{Addr: *flHost}
	for _, tmpl := range benchmarks {
		if tmpl.Skip {
			continue
		}

		b, err := tmpl.Create()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERR: %s\n", err)
			exitStatus = 1
			continue
		}

		if mustPing {
			if err := c.Ping(); err != nil {
				fmt.Fprintf(os.Stderr, "WARN: unable to ping server, waiting for server...\n")
				for {
					time.Sleep(time.Second)
					if err := c.Ping(); err == nil {
						break
					}
				}
			}
			mustPing = false
		}

		// Reseed the random number generator for each test.
		// TODO(jsternberg): Add the ability to rerun tests with different
		// seeds to try and capture potential situations that we should test.
		rand.Seed(tmpl.Seed)

		result, err := b.Run(&c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERR: %s\n", err)
			mustPing = true
			exitStatus = 1
			continue
		}
		fmt.Printf("%-*s\t%s\n", maxLen, tmpl.Name, result)
	}
	return exitStatus
}

func main() {
	os.Exit(realMain())
}
