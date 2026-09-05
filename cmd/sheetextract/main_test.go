package main

import "testing"

func TestComponentOwners(t *testing.T) {
	components := []component{
		{id: 0, area: 1000, minX: 10, minY: 10, maxX: 40, maxY: 40},
		{id: 1, area: 900, minX: 110, minY: 10, maxX: 140, maxY: 40},
		{id: 2, area: 20, minX: 45, minY: 20, maxX: 48, maxY: 23},
		{id: 3, area: 11, minX: 50, minY: 20, maxX: 51, maxY: 21},
		{id: 4, area: 100, minX: 20, minY: 72, maxX: 30, maxY: 80},
	}
	owners := componentOwners(components, components[:2])
	want := []int{0, 1, 0, -1, -1}
	for index := range want {
		if owners[index] != want[index] {
			t.Fatalf("owner %d = %d, want %d", index, owners[index], want[index])
		}
	}
}

func TestBoxDistance(t *testing.T) {
	left := component{minX: 10, minY: 10, maxX: 20, maxY: 20}
	right := component{minX: 24, minY: 15, maxX: 30, maxY: 25}
	if got, want := boxDistance(left, right), 9; got != want {
		t.Fatalf("boxDistance() = %d, want %d", got, want)
	}
}
