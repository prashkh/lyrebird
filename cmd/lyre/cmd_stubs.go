// Stubs for commands implemented in later phases. They live here so the
// package compiles incrementally.
package main

import "fmt"

func cmdSearch(args []string) error { return notImpl("search") }
func cmdUI(args []string) error     { return notImpl("ui") }

func notImpl(name string) error {
	return fmt.Errorf("%s: not implemented yet", name)
}
