//go:build !linux

package main

import "fmt"

func main() { fmt.Println("WireGuard activation delegate requires Linux; no network action executed.") }
