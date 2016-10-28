procedure for benchmark
match what go test does, try to output in the same format so that we can
use benchcmp

create database and insert data, only needs to be done once
try to force a compaction?
continue running queries until some condition
  go test does it until the benchmark takes 1 second to run, but we
  might need something like "wait for the standard deviation to go below
  a certain value with a minimum of some number of runs". we can just
  start by running the query 100 times.
