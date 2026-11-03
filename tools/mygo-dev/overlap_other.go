//go:build !darwin

package main

// windowProbe is nil where reloads keep MyGo's order: the running build quits, then the next starts.
var windowProbe func(pid int) bool
