// Package store persists Trainers and immutable Battle Results.
package store

import (
	"errors"
	"time"

	"termon.sh/internal/game"
)

// ErrNotFound means the requested Trainer or credential does not exist.
var ErrNotFound = errors.New("store: trainer not found")

// ErrAlreadyOnboarded is returned when the Trainer already completed onboarding.
var ErrAlreadyOnboarded = errors.New("store: trainer already onboarded")

// ErrCorruptSave means a stored Save envelope cannot be decoded safely.
var ErrCorruptSave = errors.New("store: corrupt Save")

// ErrSaveTooNew means a Save payload was written by a newer termond binary.
var ErrSaveTooNew = errors.New("store: Save version is newer than this binary")

// ErrResultConflict means a Battle ID was reused for a different outcome.
var ErrResultConflict = errors.New("store: conflicting Battle Result")

// ErrInvalidParty means Party slot IDs are invalid.
var ErrInvalidParty = errors.New("store: invalid party")

// ErrInvalidLoadout means Battle Loadout slugs are invalid.
var ErrInvalidLoadout = errors.New("store: invalid battle loadout")

// ErrUnknownMonster means the Monster ID is not in Collection.
var ErrUnknownMonster = errors.New("store: unknown monster")

// ErrEvolutionNotPending means AcceptEvolution was called without pending evolution.
var ErrEvolutionNotPending = errors.New("store: evolution not pending")

// BattleResult is the immutable outcome committed for one multiplayer Battle.
type BattleResult struct {
	ID          string
	Winner      string
	Loser       string
	Reason      string
	CompletedAt time.Time
}

// BattleRecord carries participation and reward intent for RecordBattleResult.
type BattleRecord struct {
	Result        BattleResult
	WinnerActive  []string
	WinnerReserve []string
	LoserActive   []string
	LoserReserve  []string
	ApplyRewards  bool
	Stats         BattleStats
}

// BattleStats is the versioned immutable summary used for player statistics.
type BattleStats struct {
	Version      int                  `json:"version"`
	StartedAt    time.Time            `json:"started_at"`
	DurationMS   int64                `json:"duration_ms"`
	Turns        int                  `json:"turns"`
	EntryPath    string               `json:"entry_path,omitempty"`
	TrainerStats []BattleTrainerStats `json:"trainers,omitempty"`
}

// BattleTrainerStats summarizes one Trainer's resolved Battle actions.
type BattleTrainerStats struct {
	TrainerID    string            `json:"trainer_id"`
	Result       string            `json:"result"`
	Moves        int               `json:"moves"`
	Misses       int               `json:"misses"`
	CriticalHits int               `json:"critical_hits"`
	Switches     int               `json:"switches"`
	DamageDealt  int               `json:"damage_dealt"`
	Faints       int               `json:"faints"`
	Monsters     []BattleMonster   `json:"monsters,omitempty"`
	MoveStats    []BattleMoveStats `json:"move_stats,omitempty"`
}

// BattleMoveStats preserves per-Move usage and resolved outcomes.
type BattleMoveStats struct {
	Slug         string `json:"slug"`
	Uses         int    `json:"uses"`
	Misses       int    `json:"misses"`
	CriticalHits int    `json:"critical_hits"`
	DamageDealt  int    `json:"damage_dealt"`
}

// BattleMonster preserves Monster identity and Species at Battle time.
type BattleMonster struct {
	ID      string `json:"id"`
	Species string `json:"species"`
}

// CaptureSpec describes an activity capture outcome.
type CaptureSpec struct {
	Species   string
	FillParty bool
}

// ActivityKind classifies a completed activity for reward pricing and
// idempotency. The typed field keeps typo'd kinds from silently pricing at
// zero XP in activityXP.
type ActivityKind string

// Published activity kinds (docs/design/xp-progression.md reward table).
const (
	KindExpedition   ActivityKind = "expedition"
	KindLesson       ActivityKind = "lesson"
	KindSparring     ActivityKind = "sparring"
	KindDailyXP      ActivityKind = "daily_xp"
	KindDailyMastery ActivityKind = "daily_mastery"
)

// Activity outcomes recorded with a result. Expedition encounters use the
// prep/target/captured/hunt_failed set; Sparring and Daily report cleared or
// mastery.
const (
	OutcomePrep1      = "prep1"
	OutcomePrep2      = "prep2"
	OutcomeTarget     = "target"
	OutcomeCaptured   = "captured"
	OutcomeHuntFailed = "hunt_failed"
	OutcomeCleared    = "cleared"
	OutcomeMastery    = "mastery"
)

// ActivityRecord is one solo activity commit request.
type ActivityRecord struct {
	Kind         ActivityKind
	NaturalKey   string
	TrainerID    string
	ActiveIDs    []string
	ReserveIDs   []string
	Outcome      string
	Capture      *CaptureSpec
	MasteryOnly  bool
	DailyParMet  bool // when Kind=KindDailyXP, also insert daily_mastery
	CompletedAt  time.Time
	SparringTier string // dojo.TierApprentice | TierRival | TierMaster for sparring kind
}

// ActivityResult is a stored solo activity row.
type ActivityResult struct {
	ID                string
	NaturalKey        string
	TrainerID         string
	Kind              ActivityKind
	CompletedAt       time.Time
	Payload           string
	CapturedMonsterID string
}

// ResultRecords contains both records after a Battle Result transaction.
type ResultRecords struct {
	Winner  *game.Save
	Loser   *game.Save
	Applied bool
}

// SessionRecord is one authenticated SSH connection.
type SessionRecord struct {
	ID           string
	TrainerID    string
	StartedAt    time.Time
	EndedAt      time.Time
	EndReason    string
	AppVersion   string
	ResumeTarget string
}

// TrainerStats contains authoritative player-visible lifetime statistics.
type TrainerStats struct {
	TrainerID      string
	CreatedAt      time.Time
	Wins           int
	Losses         int
	CurrentStreak  int
	LongestStreak  int
	CollectionSize int
	Captures       int
	Expeditions    int
	DojoClears     int
	MasteryMarks   int
	Sessions       int
	PlayTime       time.Duration
}

// WorldStats contains durable global totals. Live concurrency stays in HubStats.
type WorldStats struct {
	RegisteredTrainers  int
	CompletedBattles    int
	CompletedActivities int
}

// Store is the transactional persistence seam used by the game server.
type Store interface {
	ResolveCredential(fingerprintHash string) (*game.Trainer, error)
	CreateTrainer(fingerprintHash string) (*game.Trainer, error)
	LoadTrainer(id string) (*game.Trainer, error)
	CompleteOnboarding(id string, save *game.Save) error
	ResetTrainer(id string) error
	RecordBattleResult(BattleRecord) (ResultRecords, error)
	RecordActivityResult(ActivityRecord) (*game.Save, error)
	SetParty(trainerID string, party [3]string) (*game.Save, error)
	SetBattleLoadout(trainerID, monsterID string, moves, noticeIDs []string) (*game.Save, error)
	AcknowledgeProgressionNotices(trainerID string, noticeIDs []string) (*game.Save, error)
	SetNickname(trainerID, monsterID, nickname string) (*game.Save, error)
	AcceptEvolution(trainerID, monsterID string) (*game.Save, error)
	ActivityExists(trainerID, naturalKey string) (bool, error)
	ActivityResult(trainerID, naturalKey string) (*ActivityResult, error)
	StartSession(SessionRecord) error
	EndSession(sessionID string, endedAt time.Time, reason string) error
	TrainerStats(trainerID string) (TrainerStats, error)
	WorldStats() (WorldStats, error)
}
