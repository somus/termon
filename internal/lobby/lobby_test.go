package lobby

import (
	"fmt"
	"testing"
)

func TestJoinAndCollision(t *testing.T) {
	room := NewDojo()
	a, err := room.Join(Presence{Hash: "a", Handle: "alpha", Species: "rootkit"})
	if err != nil {
		t.Fatal(err)
	}
	if a.X == 0 || a.Y == 0 {
		t.Fatalf("spawned on a wall: %+v", a)
	}
	b, err := room.Join(Presence{Hash: "b", Handle: "bravo", Species: "aquabit"})
	if err != nil {
		t.Fatal(err)
	}
	if a.X == b.X && a.Y == b.Y {
		t.Fatal("two trainers spawned on the same tile")
	}
	if err := room.Move("a", North); err != nil {
		t.Fatalf("open north step: %v", err)
	}
}

func TestDojoFurnishings(t *testing.T) {
	room := NewDojo()
	wants := []struct {
		name     string
		x, y     int
		kind     ObjectKind
		passable bool
	}{
		{"master", 24, 10, ObjectMaster, false},
		{"gong", 20, 10, ObjectGong, false},
		{"dummy", 28, 10, ObjectDummy, false},
		{"notice board", 9, 8, ObjectNoticeBoard, false},
		{"west lantern", 18, 1, ObjectLantern, false},
		{"crest", 24, 1, ObjectCrest, false},
		{"east lantern", 30, 1, ObjectLantern, false},
		{"west bench", 7, 2, ObjectBench, false},
		{"east bench", 41, 2, ObjectBench, false},
		{"west plant", 4, 2, ObjectPlant, false},
		{"east plant", 44, 2, ObjectPlant, false},
		{"sleeper", 42, 3, ObjectSleeper, true},
	}
	for _, want := range wants {
		t.Run(want.name, func(t *testing.T) {
			obj, ok := room.ObjectAt(want.x, want.y)
			if !ok {
				t.Fatalf("object at (%d,%d) is missing", want.x, want.y)
			}
			if obj.Kind != want.kind || obj.Passable != want.passable {
				t.Fatalf("object at (%d,%d) = %+v, want kind %d passable %v", want.x, want.y, obj, want.kind, want.passable)
			}
		})
	}
	if got, want := len(room.objects), len(wants); got != want {
		t.Fatalf("object count = %d, want %d", got, want)
	}
}

func TestDojoArchitectureAndCourt(t *testing.T) {
	layout := SharedLayout()
	walls := [][2]int{{0, 6}, {Width - 1, 6}, {24, 0}, {24, Height - 1}, {24, 1}}
	for _, wall := range walls {
		if !layout.Blocked(wall[0], wall[1]) {
			t.Fatalf("wall at (%d,%d) is passable", wall[0], wall[1])
		}
	}
	if got := layout.SurfaceAt(24, 1); got != SurfaceNorthWall {
		t.Fatalf("north wall surface = %d, want %d", got, SurfaceNorthWall)
	}

	tests := []struct {
		name    string
		x, y    int
		surface SurfaceKind
		blocked bool
	}{
		{"west border", CourtMinX, CourtCenterY, SurfaceCourtBorder, false},
		{"east border", CourtMaxX, CourtCenterY, SurfaceCourtBorder, false},
		{"north border", CourtCenterX, CourtMinY, SurfaceCourtBorder, false},
		{"south border", CourtCenterX, CourtMaxY, SurfaceCourtBorder, false},
		{"west start", 20, CourtCenterY, SurfaceCourtStart, false},
		{"east start", 28, CourtCenterY, SurfaceCourtStart, false},
		{"center crest", CourtCenterX, CourtCenterY, SurfaceCourtCrest, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := layout.SurfaceAt(tt.x, tt.y); got != tt.surface {
				t.Fatalf("surface at (%d,%d) = %d, want %d", tt.x, tt.y, got, tt.surface)
			}
			if got := layout.Blocked(tt.x, tt.y); got != tt.blocked {
				t.Fatalf("blocked at (%d,%d) = %v, want %v", tt.x, tt.y, got, tt.blocked)
			}
		})
	}

	for y := CourtMinY; y <= CourtMaxY; y++ {
		for x := CourtMinX; x <= CourtMaxX; x++ {
			if _, ok := layout.ObjectAt(x, y); ok {
				t.Fatalf("fighting court contains an object at (%d,%d)", x, y)
			}
			border := x == CourtMinX || x == CourtMaxX || y == CourtMinY || y == CourtMaxY
			if border && layout.Blocked(x, y) {
				t.Fatalf("court border at (%d,%d) is blocked", x, y)
			}
		}
	}

	for _, y := range []int{CourtMinY - 1, CourtMaxY + 1} {
		for x := CourtMinX; x <= CourtMaxX; x++ {
			if got := layout.SurfaceAt(x, y); got != SurfaceTatami {
				t.Fatalf("walkway surface at (%d,%d) = %d, want tatami", x, y, got)
			}
			if layout.Blocked(x, y) {
				t.Fatalf("walkway at (%d,%d) is blocked", x, y)
			}
		}
	}
}

func TestDojoOpenFloorAndReachability(t *testing.T) {
	room := NewDojo()
	for y := 2; y < Height-1; y++ {
		for x := 1; x < Width-1; x++ {
			obj, exists := room.ObjectAt(x, y)
			if room.Blocked(x, y) != (exists && !obj.Passable) {
				t.Fatalf("unexpected collision at (%d,%d)", x, y)
			}
		}
	}

	start := [2]int{MasterX, MasterY + 1}
	reached := map[[2]int]bool{start: true}
	queue := [][2]int{start}
	for len(queue) > 0 {
		point := queue[0]
		queue = queue[1:]
		for _, step := range [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} {
			next := [2]int{point[0] + step[0], point[1] + step[1]}
			if !reached[next] && !room.Blocked(next[0], next[1]) {
				reached[next] = true
				queue = append(queue, next)
			}
		}
	}
	for i := range Capacity {
		p, err := room.Join(Presence{Hash: fmt.Sprintf("trainer-%02d", i)})
		if err != nil {
			t.Fatal(err)
		}
		if !reached[[2]int{p.X, p.Y}] {
			t.Fatalf("spawn (%d,%d) cannot reach Sable", p.X, p.Y)
		}
	}
	for point := range room.objects {
		accessible := false
		for _, step := range [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} {
			accessible = accessible || reached[[2]int{point[0] + step[0], point[1] + step[1]}]
		}
		if !accessible {
			t.Fatalf("landmark at %v cannot be approached from the entrance", point)
		}
	}
	for x := 1; x < Width-1; x++ {
		if _, exists := room.ObjectAt(x, Height-2); exists || room.Blocked(x, Height-2) {
			t.Fatalf("entrance is obstructed at x=%d", x)
		}
	}
}

func TestTrainerCanCrossCourtBorder(t *testing.T) {
	room := NewDojo()
	p := Presence{Hash: "trainer", Handle: "trainer", X: CourtMinX - 1, Y: CourtCenterY}
	if err := room.Place(p); err != nil {
		t.Fatalf("place Trainer outside court: %v", err)
	}
	if err := room.Move(p.Hash, East); err != nil {
		t.Fatalf("move Trainer onto court border: %v", err)
	}
	if err := room.Move(p.Hash, East); err != nil {
		t.Fatalf("move Trainer through court border: %v", err)
	}
}

func TestFullEntranceCanMoveNorth(t *testing.T) {
	room := NewDojo()
	for i := range Capacity {
		hash := fmt.Sprintf("trainer-%02d", i)
		if _, err := room.Join(Presence{Hash: hash, Handle: hash}); err != nil {
			t.Fatalf("join %s: %v", hash, err)
		}
	}
	for i := range Capacity {
		hash := fmt.Sprintf("trainer-%02d", i)
		if err := room.Move(hash, North); err != nil {
			t.Fatalf("move %s north from entrance: %v", hash, err)
		}
	}
	if err := room.Move("trainer-00", North); err != nil {
		t.Fatalf("move first spawn into west aisle: %v", err)
	}
}

func TestWallsBlockMovement(t *testing.T) {
	room := NewDojo()
	if _, err := room.Join(Presence{Hash: "a", Handle: "a", Species: "rootkit"}); err != nil {
		t.Fatal(err)
	}
	p, _ := room.Get("a")
	// Entrance is on the south row; south is a wall.
	if p.Y != Height-2 {
		t.Fatalf("spawn y = %d, want %d", p.Y, Height-2)
	}
	if err := room.Move("a", South); err == nil {
		t.Fatal("expected wall to block south")
	}
}

func TestAdjacentChallengeTarget(t *testing.T) {
	room := NewDojo()
	if err := room.Place(Presence{Hash: "a", Handle: "a", Species: "rootkit", X: 10, Y: 6}); err != nil {
		t.Fatal(err)
	}
	if err := room.Place(Presence{Hash: "b", Handle: "b", Species: "emberbyte", X: 11, Y: 6}); err != nil {
		t.Fatal(err)
	}
	n := room.Adjacent("a")
	if len(n) != 1 || n[0].Hash != "b" {
		t.Fatalf("adjacent = %+v, want b", n)
	}
	if len(room.Adjacent("b")) != 1 {
		t.Fatal("adjacency should be symmetric")
	}
}

func TestInBattleCannotMove(t *testing.T) {
	room := NewDojo()
	if _, err := room.Join(Presence{Hash: "a", Handle: "a", Species: "rootkit"}); err != nil {
		t.Fatal(err)
	}
	room.Set("a", func(p *Presence) { p.InBattle = true })
	if err := room.Move("a", North); err == nil {
		t.Fatal("expected in-battle trainer to be immobile")
	}
}

func TestPassableObjectAllowsTrainerAndSurfacesDiscovery(t *testing.T) {
	room := NewDojo()
	if err := room.Place(Presence{Hash: "a", Handle: "alpha", X: 41, Y: 3}); err != nil {
		t.Fatal(err)
	}
	if err := room.Move("a", East); err != nil {
		t.Fatalf("move onto passable sleeper: %v", err)
	}
	p, _ := room.Get("a")
	obj, ok := room.ObjectAt(p.X, p.Y)
	if !ok || obj.Kind != ObjectSleeper || !obj.Passable {
		t.Fatalf("object under Trainer = %+v ok=%v", obj, ok)
	}
	if room.Context("a") == "" {
		t.Fatal("passable discovery did not surface context")
	}
}

func TestPeopleAndImpassableObjectsBlockMovement(t *testing.T) {
	room := NewDojo()
	if err := room.Place(Presence{Hash: "a", X: MasterX - 2, Y: MasterY}); err != nil {
		t.Fatal(err)
	}
	if err := room.Move("a", East); err != nil {
		t.Fatal(err)
	}
	if !room.NearMaster("a") {
		t.Fatal("Trainer next to Master should be in practice range")
	}
	if got := room.Context("a"); got != "Master Sable waits. Enter for Sparring and Daily." {
		t.Fatalf("master context = %q", got)
	}
	if err := room.Move("a", East); err == nil {
		t.Fatal("expected Master tile to block movement")
	}

	room.Leave("a")
	if _, err := room.Join(Presence{Hash: "spawned"}); err != nil {
		t.Fatal(err)
	}
	if err := room.PlaceBesideMaster("spawned"); err != nil {
		t.Fatal(err)
	}
	if !room.NearMaster("spawned") {
		t.Fatal("PlaceBesideMaster should seat next to Master Sable")
	}

	room.Leave("a")
	if err := room.Place(Presence{Hash: "a", X: NoticeBoardX - 1, Y: NoticeBoardY}); err != nil {
		t.Fatal(err)
	}
	if !room.NearNoticeBoard("a") {
		t.Fatal("Trainer next to notice board should be in range")
	}
	if got := room.Context("a"); got != "Signal Board: Enter for Expeditions." {
		t.Fatalf("notice board context = %q", got)
	}
	if err := room.Move("a", East); err == nil {
		t.Fatal("expected notice board tile to block movement")
	}

	room.Leave("a")
	if err := room.Place(Presence{Hash: "a", X: 10, Y: 10}); err != nil {
		t.Fatal(err)
	}
	if err := room.Place(Presence{Hash: "b", X: 11, Y: 10}); err != nil {
		t.Fatal(err)
	}
	if err := room.Move("a", East); err == nil {
		t.Fatal("expected another Trainer to block movement")
	}
}

func TestOutOfBoundsPlacementIsRejected(t *testing.T) {
	room := NewDojo()
	if err := room.Place(Presence{Hash: "a", X: -1, Y: 1}); err == nil {
		t.Fatal("expected out-of-bounds placement to fail")
	}
}
