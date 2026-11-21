package main

import (
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Input arrives between MyGo build passes. Its handlers must use retained
// geometry or the thumb can paint correctly but never send the browser a seek.
func TestPinterestRailSeeksAfterBuild(t *testing.T) {
	b := &pinterestBrowser{webScroll: webScroll{percent: 0, view: 25}, mailbox: &scrollMailbox{}, scrollWake: make(chan struct{}, 1)}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Fill().Children(func() { b.scrollbar(c) })
	}, 200, 200)
	r, ok := tt.Find("Pinterest 网页滚动条")
	if !ok {
		t.Fatal("missing rail")
	}
	tt.Wheel(r.X+4, r.Y+16, 0, 60)
	v, err := strconv.ParseFloat(b.mailbox.take(), 64)
	if err != nil || v < 9.99 || v > 10.01 {
		t.Fatalf("wheel did not scroll a 600 pt range by 60 pt: %v, %v", v, err)
	}
}
