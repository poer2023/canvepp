package main

import (
	"math"
	"testing"
)

func TestConnectionCurveKeepsSubpixelShapeAndEndpoints(t *testing.T) {
	for _, v := range [][5]float32{{0, 0, 800, 600, 360}, {900, 100, 100, 100, 180}, {-12000, -8000, 16000, 9000, 12600}, {100, 40, 105, 640, 56}} {
		var buf [257][2]float32
		points := connectionPoints(buf[:0], v[0], v[1], v[2], v[3], v[4])
		if len(points) > len(buf) || points[0] != [2]float32{v[0], v[1]} || points[len(points)-1] != [2]float32{v[2], v[3]} {
			t.Fatal("curve endpoints changed")
		}
		for i := 0; i <= 1000; i++ {
			tt := float64(i) / 1000
			u := 1 - tt
			x := u*u*u*float64(v[0]) + 3*u*u*tt*float64(v[0]+v[4]) + 3*u*tt*tt*float64(v[2]-v[4]) + tt*tt*tt*float64(v[2])
			y := u*u*u*float64(v[1]) + 3*u*u*tt*float64(v[1]) + 3*u*tt*tt*float64(v[3]) + tt*tt*tt*float64(v[3])
			nearest := math.Inf(1)
			for j := 1; j < len(points); j++ {
				a, b := points[j-1], points[j]
				dx, dy := float64(b[0]-a[0]), float64(b[1]-a[1])
				t := 0.0
				if dx*dx+dy*dy > 0 {
					t = math.Max(0, math.Min(1, ((x-float64(a[0]))*dx+(y-float64(a[1]))*dy)/(dx*dx+dy*dy)))
				}
				nearest = math.Min(nearest, math.Hypot(x-float64(a[0])-t*dx, y-float64(a[1])-t*dy))
			}
			if nearest > .4 {
				t.Fatalf("connection deviates %.3f pixels", nearest)
			}
		}
	}
}

func TestConnectionTessellationDoesNotAllocate(t *testing.T) {
	var buf [257][2]float32
	if n := testing.AllocsPerRun(100, func() { connectionPoints(buf[:0], -12000, -8000, 16000, 9000, 12600) }); n != 0 {
		t.Fatalf("curve allocated %v times", n)
	}
}
