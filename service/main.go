package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var prefix = "[" + strings.ToUpper(filepath.Base(os.Args[0])) + "]:"

func service(done chan int, countAt int, delay time.Duration) {
	defer close(done)
	var delayFn = func(duration time.Duration) {}
	if delay > 0 {
		delayFn = time.Sleep
	}

	for i := 0; i < countAt; i++ {
		log.Println(prefix, "->", i)
		delayFn(delay)
	}
}

func do(countAt int, delay time.Duration) {
	var (
		done = make(chan int, 1)
		sigs = make(chan os.Signal, 1)
	)
	// This goroutine executes a blocking receive for
	// signals. When it gets one it'll print it out
	// and then notify the program that it can finish.
	go func() {
		sig := <-sigs
		println()
		log.Println(prefix, "received signal:", sig)
		done <- 0
	}()

	// `signal.Notify` registers the given channel to
	// receive notifications of the specified signals.
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go service(done, countAt, delay)

	// The program will wait here until it gets the
	// expected signal (as indicated by the goroutine
	// above sending a value on `done`) and then exit.
	log.Println(prefix, "awaiting signal")
	status := <-done
	log.Printf(prefix+" Exit status: %d.", status)
	os.Exit(status)
}

func main() {
	var (
		countAt = flag.Int("to", 10, "count to value")
		delay   = flag.Duration("delay", time.Second, "duration (see time.Duration#String). Example: 50ms, 10s etc.")
	)

	flag.Parse()

	do(*countAt, *delay)
}
