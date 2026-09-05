// Package lobby owns the Dojo room: positions, collision, and adjacency.
package lobby

import (
	"errors"
	"sync"
	"time"
)

// Dojo room size in tiles.
const (
	Width    = 48
	Height   = 14
	Capacity = 32

	MasterX = 24
	MasterY = 10

	NoticeBoardX = 9
	NoticeBoardY = 8

	CourtMinX    = 14
	CourtMaxX    = 34
	CourtMinY    = 4
	CourtMaxY    = 8
	CourtCenterX = 24
	CourtCenterY = 6
)

// SurfaceKind identifies the static material beneath a tile.
type SurfaceKind int

// Dojo floor and architecture surfaces.
const (
	SurfaceTatami SurfaceKind = iota
	SurfaceCourt
	SurfaceCourtBorder
	SurfaceCourtStart
	SurfaceCourtCrest
	SurfaceNorthWall
	SurfaceWall
)

// ObjectKind identifies a fixed Dojo landmark without coupling the world to
// its terminal rendering.
type ObjectKind int

// Dojo landmark kinds, in draw order.
const (
	ObjectPillar ObjectKind = iota
	ObjectMaster
	ObjectGong
	ObjectDummy
	ObjectSign
	ObjectScroll
	ObjectFloorboard
	ObjectSleeper
	ObjectBanner
	ObjectWallScroll
	ObjectLantern
	ObjectCrest
	ObjectTrophyCase
	ObjectBadgeDisplay
	ObjectPlant
	ObjectBench
	ObjectCubbies
	ObjectGearRack
	ObjectPracticePads
	ObjectWaterUrn
	ObjectRecordTerminal
	ObjectNoticeBoard
	ObjectFirstAid
	ObjectTowelStation
)

// Object is a fixed landmark. Passable objects remain on the floor layer, so
// a Trainer standing on their tile renders over them.
type Object struct {
	Kind      ObjectKind
	Label     string
	Passable  bool
	Discovery string
}

// Dir is a one-tile step.
type Dir int

// Cardinal steps.
const (
	North Dir = iota
	South
	East
	West
)

// Presence is one Trainer standing in the room.
type Presence struct {
	Hash       string
	Handle     string
	Species    string
	X, Y       int
	InBattle   bool
	InQueue    bool
	Emote      string
	EmoteUntil time.Time
}

// Room is the server-owned Dojo grid.
//
// Not safe for concurrent use: every caller must hold the Hub mutex (the
// server package does this everywhere; see internal/server/hub.go). For
// lock-free reads of the static geometry, use SharedLayout.
type Room struct {
	blocked map[[2]int]bool
	objects map[[2]int]Object
	spawns  [][2]int
	byHash  map[string]Presence
}

// NewDojo builds a 48x14 hall with walls, pillars, and 32 entrance spawns.
func NewDojo() *Room {
	l := buildDojoLayout()
	return &Room{
		blocked: l.blocked,
		objects: l.objects,
		spawns:  l.spawns,
		byHash:  map[string]Presence{},
	}
}

// DojoLayout is the immutable geometry of the standard dojo: walls, pillars,
// objects, and spawn tiles. It carries no trainer state, so a single shared
// instance is safe for concurrent readers.
type DojoLayout struct {
	blocked  map[[2]int]bool
	objects  map[[2]int]Object
	surfaces map[[2]int]SurfaceKind
	spawns   [][2]int
}

var (
	dojoOnce   sync.Once
	dojoShared *DojoLayout
)

// SharedLayout returns the shared immutable dojo geometry. Render paths
// should prefer this over constructing a throwaway Room per frame.
func SharedLayout() *DojoLayout {
	dojoOnce.Do(func() { dojoShared = buildDojoLayout() })
	return dojoShared
}

// Blocked reports whether the tile is a wall or otherwise impassable.
func (l *DojoLayout) Blocked(x, y int) bool {
	return l.blocked[[2]int{x, y}]
}

// ObjectAt returns any object occupying the tile.
func (l *DojoLayout) ObjectAt(x, y int) (Object, bool) {
	obj, ok := l.objects[[2]int{x, y}]
	return obj, ok
}

// SurfaceAt returns the floor or architectural material at a tile.
func (l *DojoLayout) SurfaceAt(x, y int) SurfaceKind {
	return l.surfaces[[2]int{x, y}]
}

func buildDojoLayout() *DojoLayout {
	l := &DojoLayout{
		blocked:  map[[2]int]bool{},
		objects:  map[[2]int]Object{},
		surfaces: map[[2]int]SurfaceKind{},
	}
	for x := range Width {
		l.blocked[[2]int{x, 0}] = true
		l.blocked[[2]int{x, Height - 1}] = true
		l.surfaces[[2]int{x, 0}] = SurfaceWall
		l.surfaces[[2]int{x, Height - 1}] = SurfaceWall
	}
	for y := range Height {
		l.blocked[[2]int{0, y}] = true
		l.blocked[[2]int{Width - 1, y}] = true
		l.surfaces[[2]int{0, y}] = SurfaceWall
		l.surfaces[[2]int{Width - 1, y}] = SurfaceWall
	}
	for x := 1; x < Width-1; x++ {
		l.blocked[[2]int{x, 1}] = true
		l.surfaces[[2]int{x, 1}] = SurfaceNorthWall
	}
	addDojoCourt(l)
	for _, p := range [][2]int{{12, 4}, {12, 8}, {35, 4}, {35, 8}, {6, 6}} {
		addLayoutObject(l, p[0], p[1], Object{Kind: ObjectPillar})
	}
	addLayoutObject(l, MasterX, MasterY, Object{Kind: ObjectMaster, Label: "Master Sable"})
	addLayoutObject(l, 20, 10, Object{Kind: ObjectGong, Label: "practice gong"})
	addLayoutObject(l, 28, 10, Object{Kind: ObjectDummy, Label: "training dummy"})
	addLayoutObject(l, 15, 12, Object{Kind: ObjectSign, Label: "MASTER ->", Passable: true})
	addLayoutObject(l, 33, 12, Object{Kind: ObjectSign, Label: "<- MASTER", Passable: true})
	addLayoutObject(l, 18, 11, Object{
		Kind: ObjectScroll, Passable: true,
		Discovery: "The old scroll reads: patience wins turns that speed cannot.",
	})
	addLayoutObject(l, 30, 11, Object{
		Kind: ObjectFloorboard, Passable: true,
		Discovery: "A loose board clicks twice. Something underneath clicks back.",
	})
	addLayoutObject(l, 42, 8, Object{
		Kind: ObjectSleeper, Passable: true,
		Discovery: "A tiny Chippunk snores in machine code, then pretends it was awake.",
	})
	addDojoFurnishings(l)
	for x := 8; x < 8+Capacity; x++ {
		l.spawns = append(l.spawns, [2]int{x, Height - 2})
	}
	return l
}

func addDojoCourt(l *DojoLayout) {
	for y := CourtMinY; y <= CourtMaxY; y++ {
		for x := CourtMinX; x <= CourtMaxX; x++ {
			point := [2]int{x, y}
			l.surfaces[point] = SurfaceCourt
			border := x == CourtMinX || x == CourtMaxX || y == CourtMinY || y == CourtMaxY
			if border {
				l.surfaces[point] = SurfaceCourtBorder
			}
		}
	}
	l.surfaces[[2]int{20, CourtCenterY}] = SurfaceCourtStart
	l.surfaces[[2]int{28, CourtCenterY}] = SurfaceCourtStart
	l.surfaces[[2]int{CourtCenterX, CourtCenterY}] = SurfaceCourtCrest
}

func addDojoFurnishings(l *DojoLayout) {
	addLayoutObject(l, 5, 1, Object{
		Kind:      ObjectBanner,
		Discovery: "The west victory banner is faded from years of morning light.",
	})
	addLayoutObject(l, 11, 1, Object{
		Kind:      ObjectWallScroll,
		Discovery: "The founding scroll reads: leave your pride at the threshold.",
	})
	addLayoutObject(l, 18, 1, Object{Kind: ObjectLantern, Label: "west wall lantern"})
	addLayoutObject(l, 24, 1, Object{Kind: ObjectCrest, Label: "Dojo crest"})
	addLayoutObject(l, 30, 1, Object{Kind: ObjectLantern, Label: "east wall lantern"})
	addLayoutObject(l, 37, 1, Object{
		Kind:      ObjectWallScroll,
		Discovery: "The conduct scroll reads: teach what you learn; test what you teach.",
	})
	addLayoutObject(l, 43, 1, Object{
		Kind:      ObjectBanner,
		Discovery: "The east victory banner records the hall's longest winning streak.",
	})

	addLayoutObject(l, 3, 2, Object{Kind: ObjectTrophyCase, Label: "trophy cabinet"})
	addLayoutObject(l, 7, 2, Object{Kind: ObjectBadgeDisplay, Label: "badge display"})
	addLayoutObject(l, 5, 4, Object{Kind: ObjectPracticePads, Label: "practice pads"})
	addLayoutObject(l, 9, 4, Object{Kind: ObjectWaterUrn, Label: "water urn"})
	addLayoutObject(l, 3, 5, Object{Kind: ObjectPlant, Label: "west plant"})
	addLayoutObject(l, 8, 6, Object{Kind: ObjectPlant, Label: "practice plant"})
	addLayoutObject(l, 4, 7, Object{Kind: ObjectRecordTerminal, Label: "record terminal"})
	addLayoutObject(l, 9, 8, Object{Kind: ObjectNoticeBoard, Label: "notice board"})
	addLayoutObject(l, 6, 9, Object{Kind: ObjectGearRack, Label: "padded staff rack"})
	addLayoutObject(l, 7, 10, Object{Kind: ObjectCubbies, Label: "west shoe cubbies"})

	addLayoutObject(l, 44, 2, Object{Kind: ObjectBench, Label: "east spectator bench"})
	addLayoutObject(l, 38, 2, Object{Kind: ObjectPlant, Label: "north-east plant"})
	addLayoutObject(l, 39, 4, Object{Kind: ObjectTowelStation, Label: "towel station"})
	addLayoutObject(l, 43, 4, Object{Kind: ObjectFirstAid, Label: "first-aid cabinet"})
	addLayoutObject(l, 45, 5, Object{Kind: ObjectPlant, Label: "recovery plant"})
	addLayoutObject(l, 43, 7, Object{Kind: ObjectGearRack, Label: "loaner gear"})
	addLayoutObject(l, 38, 7, Object{Kind: ObjectPlant, Label: "east aisle plant"})
	addLayoutObject(l, 42, 9, Object{Kind: ObjectCubbies, Label: "east shoe cubbies"})
	addLayoutObject(l, 45, 9, Object{Kind: ObjectPlant, Label: "south-east plant"})
	addLayoutObject(l, 38, 10, Object{Kind: ObjectBench, Label: "recovery bench"})

	addLayoutObject(l, 13, 2, Object{Kind: ObjectPlant, Label: "north-west court plant"})
	addLayoutObject(l, 17, 2, Object{Kind: ObjectBench, Label: "north-west court bench"})
	addLayoutObject(l, 31, 2, Object{Kind: ObjectBench, Label: "north-east court bench"})
	addLayoutObject(l, 35, 2, Object{Kind: ObjectPlant, Label: "north-east court plant"})
	addLayoutObject(l, 13, 10, Object{Kind: ObjectPlant, Label: "south-west court plant"})
	addLayoutObject(l, 17, 10, Object{Kind: ObjectBench, Label: "south-west court bench"})
	addLayoutObject(l, 31, 10, Object{Kind: ObjectBench, Label: "south-east court bench"})
	addLayoutObject(l, 35, 10, Object{Kind: ObjectPlant, Label: "south-east court plant"})
}

func addLayoutObject(l *DojoLayout, x, y int, o Object) {
	l.objects[[2]int{x, y}] = o
	if !o.Passable {
		l.blocked[[2]int{x, y}] = true
	}
}

// Join places a Trainer on a free entrance tile.
func (r *Room) Join(p Presence) (Presence, error) {
	if p.Hash == "" {
		return Presence{}, errors.New("lobby: empty hash")
	}
	if _, ok := r.byHash[p.Hash]; ok {
		return r.byHash[p.Hash], nil
	}
	for _, s := range r.spawns {
		if r.occupied(s[0], s[1]) {
			continue
		}
		p.X, p.Y = s[0], s[1]
		r.byHash[p.Hash] = p
		return p, nil
	}
	return Presence{}, errors.New("lobby: entrance is full")
}

// Leave removes a Trainer from the room.
func (r *Room) Leave(hash string) {
	delete(r.byHash, hash)
}

// Place puts a Trainer on an explicit tile. Used by tests and reconnect.
func (r *Room) Place(p Presence) error {
	if r.Blocked(p.X, p.Y) || r.occupied(p.X, p.Y) {
		return errors.New("lobby: cannot place")
	}
	r.byHash[p.Hash] = p
	return nil
}

// Get returns a Trainer's presence.
func (r *Room) Get(hash string) (Presence, bool) {
	p, ok := r.byHash[hash]
	return p, ok
}

// Set flags InBattle / InQueue without moving the Trainer.
func (r *Room) Set(hash string, fn func(*Presence)) {
	p, ok := r.byHash[hash]
	if !ok {
		return
	}
	fn(&p)
	r.byHash[hash] = p
}

// Move steps one tile if the destination is free and not blocked.
func (r *Room) Move(hash string, d Dir) error {
	p, ok := r.byHash[hash]
	if !ok {
		return errors.New("lobby: unknown trainer")
	}
	if p.InBattle || p.InQueue {
		return errors.New("lobby: cannot move now")
	}
	x, y := p.X, p.Y
	switch d {
	case North:
		y--
	case South:
		y++
	case East:
		x++
	case West:
		x--
	}
	if r.Blocked(x, y) || r.occupied(x, y) {
		return errors.New("lobby: blocked")
	}
	p.X, p.Y = x, y
	r.byHash[hash] = p
	return nil
}

// Adjacent returns orthogonally neighboring Trainers.
func (r *Room) Adjacent(hash string) []Presence {
	p, ok := r.byHash[hash]
	if !ok {
		return nil
	}
	var out []Presence
	for _, q := range r.byHash {
		if q.Hash == hash {
			continue
		}
		if abs(q.X-p.X)+abs(q.Y-p.Y) == 1 {
			out = append(out, q)
		}
	}
	return out
}

// Snapshot copies everyone currently in the room.
func (r *Room) Snapshot() []Presence {
	out := make([]Presence, 0, len(r.byHash))
	for _, p := range r.byHash {
		out = append(out, p)
	}
	return out
}

// Blocked reports a collision tile (wall or pillar).
func (r *Room) Blocked(x, y int) bool {
	if x < 0 || y < 0 || x >= Width || y >= Height {
		return true
	}
	return r.blocked[[2]int{x, y}]
}

// ObjectAt returns the fixed landmark on a tile.
func (r *Room) ObjectAt(x, y int) (Object, bool) {
	obj, ok := r.objects[[2]int{x, y}]
	return obj, ok
}

// NearMaster reports whether a Trainer can practice with the Dojo Master.
func (r *Room) NearMaster(hash string) bool {
	p, ok := r.byHash[hash]
	if !ok {
		return false
	}
	return abs(p.X-MasterX)+abs(p.Y-MasterY) == 1
}

// PlaceBesideMaster moves an already-seated Trainer onto a free tile
// orthogonally adjacent to Master Sable.
func (r *Room) PlaceBesideMaster(hash string) error {
	p, ok := r.byHash[hash]
	if !ok {
		return errors.New("lobby: unknown trainer")
	}
	if abs(p.X-MasterX)+abs(p.Y-MasterY) == 1 {
		return nil
	}
	spots := [][2]int{
		{MasterX, MasterY + 1},
		{MasterX - 1, MasterY},
		{MasterX + 1, MasterY},
		{MasterX, MasterY - 1},
	}
	for _, s := range spots {
		if r.Blocked(s[0], s[1]) {
			continue
		}
		taken := false
		for _, q := range r.byHash {
			if q.Hash != hash && q.X == s[0] && q.Y == s[1] {
				taken = true
				break
			}
		}
		if taken {
			continue
		}
		p.X, p.Y = s[0], s[1]
		r.byHash[hash] = p
		return nil
	}
	return errors.New("lobby: no space beside Master")
}

// NearNoticeBoard reports whether a Trainer can open the Signal Board.
func (r *Room) NearNoticeBoard(hash string) bool {
	p, ok := r.byHash[hash]
	if !ok {
		return false
	}
	return abs(p.X-NoticeBoardX)+abs(p.Y-NoticeBoardY) == 1
}

// Context returns the proximity affordance or discovery at a Trainer's tile.
func (r *Room) Context(hash string) string {
	p, ok := r.byHash[hash]
	if !ok {
		return ""
	}
	if r.NearMaster(hash) {
		return "Master Sable waits. Enter for Sparring and Daily."
	}
	if r.NearNoticeBoard(hash) {
		return "Signal Board: Enter for Expeditions."
	}
	if obj, ok := r.ObjectAt(p.X, p.Y); ok {
		return obj.Discovery
	}
	return ""
}

func (r *Room) occupied(x, y int) bool {
	for _, p := range r.byHash {
		if p.X == x && p.Y == y {
			return true
		}
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
