package main

import "flag"

// Options are the command-line options.
type Options struct {
	DSN string
}

func parseFlags() Options {
	dsn := flag.String("dsn", "memory", "database")
	flag.Parse()
	return Options{DSN: *dsn}
}
