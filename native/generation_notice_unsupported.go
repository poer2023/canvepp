//go:build !windows && (!darwin || !cgo)

package main

func generationNoticeScheme(string) string { return "" }

func deliverGenerationNotice(_ *Store, _ generationNotice) error { return nil }
