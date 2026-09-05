//go:build unix

package main

import "syscall"

// restrictFileMode makes every file termond creates owner-only: the SQLite
// database, its WAL and SHM siblings, and any log output.
func restrictFileMode() {
	_ = syscall.Umask(0o077)
}
