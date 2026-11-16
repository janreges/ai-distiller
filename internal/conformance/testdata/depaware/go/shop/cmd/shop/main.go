package main

import (
	"fmt"

	klog "example.com/kit/log"
	"example.com/shop/store"
)

func main() {
	opts := parseFlags()
	s := store.Open(opts.DSN)
	klog.Info(fmt.Sprintf("%d products", s.Count()))
}
