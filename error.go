package main

type exitReason int

const (
	exCommandline exitReason = 1 + iota
	exWatcher
	exFsevent
)
