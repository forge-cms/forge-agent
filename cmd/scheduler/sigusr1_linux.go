package main

import (
	"os"
	"os/signal"
	"syscall"
)

func notifyUSR1(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGUSR1)
}
