package main

import (
	"flag"
	"fmt"
	"os"
)

type commandOptions struct {
	command, id, requestID, address, userID string
	wait, down, gpu                         bool
	cpu, memory, disk                       int
}

func parseOptions() (commandOptions, error) {
	opts := commandOptions{command: os.Args[1]}
	command := opts.command
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.StringVar(&opts.id, "id", "", "VM ID or operation UUID")
	flags.StringVar(&opts.requestID, "request-id", "", "idempotency UUID")
	flags.BoolVar(&opts.wait, "wait", false, "wait for operation completion")
	flags.StringVar(&opts.address, "address", env("TEMPORAL_ADDRESS", "localhost:7233"), "Temporal address")
	if command == "create" {
		flags.IntVar(&opts.cpu, "cpu", 4, "vCPU")
		flags.IntVar(&opts.memory, "memory", 8192, "RAM MiB")
		flags.IntVar(&opts.disk, "disk", 20, "disk GiB")
		flags.BoolVar(&opts.gpu, "gpu", false, "whole GPU")
	}
	if command == "create" || command == "start" || command == "stop" || command == "delete" || command == "cleanup" {
		flags.StringVar(&opts.userID, "user", "unknown", "инициатор команды (без проверки прав)")
	}
	args := os.Args[2:]
	if command == "migrate" && len(args) > 0 && args[0] == "down" {
		opts.down = true
		args = args[1:]
	}
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if flags.NArg() != 0 {
		return opts, fmt.Errorf("unexpected arguments")
	}
	return opts, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
