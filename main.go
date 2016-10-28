package main

import (
	"flag"
	"fmt"
	"os"
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
	return fmt.Sprintf("Benchmark%s_%s", strings.Title(tmpl.Type), strings.Join(parts, ""))
}

func main() {
	flHost := flag.String("H", "", "host of influxdb instance")
	flConfig := flag.String("config", "", "config file to load")
	flag.Parse()

	cfg := Config{}
	if _, err := toml.DecodeFile(*flConfig, &cfg); err != nil {
		panic(err)
	}

	maxLen := 0
	benchmarks := make([]Template, 0, len(cfg.Benchmarks))
	for _, c := range cfg.Benchmarks {
		tmpl := Template{}
		if err := mapstructure.Decode(c, &tmpl); err != nil {
			fmt.Println("ERR:", err)
			os.Exit(1)
		}
		tmpl.Name = benchName(tmpl)
		if len(tmpl.Name) > maxLen {
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
		b, err := tmpl.Create()
		if err != nil {
			fmt.Println("ERR:", err)
			os.Exit(1)
		}

		result, err := b.Run(&c)
		if err != nil {
			fmt.Println("ERR:", err)
			os.Exit(1)
		}
		fmt.Printf("%-*s\t%s\n", maxLen, tmpl.Name, result)
	}
}
