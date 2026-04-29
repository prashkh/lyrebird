// Stubs for commands implemented in later phases. They live here so the
// package compiles incrementally.
package main

import "fmt"

func cmdSessions(args []string) error { return notImpl("sessions") }
func cmdSession(args []string) error  { return notImpl("session") }
func cmdSearch(args []string) error   { return notImpl("search") }
func cmdHook(args []string) error     { return notImpl("hook") }
func cmdHandoff(args []string) error  { return notImpl("handoff") }
func cmdUI(args []string) error       { return notImpl("ui") }
func cmdInstallHook(args []string) error {
	return notImpl("install-hook")
}

func notImpl(name string) error {
	return fmt.Errorf("%s: not implemented yet", name)
}
