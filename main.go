package main

import (
	"flag"
	"fmt"
	"os"
	"regexp/syntax"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	influxdb "github.com/influxdata/influxdb-client"
)

// [[tag]]
// cardinality = 100
// template = "server[a-z]"

func opProcess(re *syntax.Regexp, keys []string) []string {
	switch re.Op {
	case syntax.OpLiteral:
		return opLiteral(re, keys)
	}
	return keys
}

func opLiteral(re *syntax.Regexp, keys []string) []string {
	if len(keys) == 0 {
		return []string{string(re.Rune)}
	}
	for i, s := range keys {
		keys[i] = s + string(re.Rune)
	}
	return keys
}

func opCharClass(re *syntax.Regexp, keys []string) []string {
	if len(keys) == 0 {
		keys = make([]string, 0, len(re.Rune))
		for _, r := range re.Rune {
			keys = append(keys, string(r))
		}
		return keys
	}
	return nil

	//newKeys := make([]string, 0, len(keys)*
}

func opConcat(re *syntax.Regexp, keys []string) []string {
	for _, e := range re.Sub {
		keys = opProcess(e, keys)
	}
	return keys
}

const configFile = `
[benchmark.write_sparse_data]
	type = "write"
`

func benchName(name string) string {
	parts := strings.Split(name, "_")
	for i, part := range parts {
		parts[i] = strings.Title(part)
	}
	return fmt.Sprintf("Benchmark%s", strings.Join(parts, ""))
}

func main() {
	flHost := flag.String("H", "", "host of influxdb instance")
	flag.Parse()

	cfg := Config{}
	if _, err := toml.Decode(configFile, &cfg); err != nil {
		panic(err)
	}

	maxLen := 0
	keys := make([]string, 0, len(cfg.Benchmarks))
	for name := range cfg.Benchmarks {
		if n := len(benchName(name)); n > maxLen {
			maxLen = n
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)

	c := influxdb.Client{
		Addr: *flHost,
	}
	for _, key := range keys {
		bt := cfg.Benchmarks[key]
		name := benchName(key)

		b, err := bt.Create()
		if err != nil {
			fmt.Println("ERR:", err)
			os.Exit(1)
		}

		result, err := b.Run(&c)
		if err != nil {
			fmt.Println("ERR:", err)
			os.Exit(1)
		}
		fmt.Printf("%-*s\t%s\n", maxLen, name, result)
	}
}
