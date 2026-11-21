//go:build !pinterestqa

package main

import (
	"context"
)

const pinterestAppName = "Canvepp Native"

func pinterestVaultKey(dir string) string { return "pinterest." + digest([]byte(dir))[:16] }

func pinterestAcceptanceSetup(*pinterestState) {}
func pinterestAcceptanceDataDir(string) error  { return nil }
func downloadPinterestPicture(ctx context.Context, raw string) ([]byte, error) {
	return downloadPin(ctx, raw)
}
