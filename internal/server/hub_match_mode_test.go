package server

import (
	"sync"
	"testing"

	"termon.sh/internal/dojo"
)

func TestMatchModePushBattleSynchronizesDecision(t *testing.T) {
	tests := []struct {
		name  string
		build func() (matchMode, *soloMode)
	}{
		{
			name: "solo",
			build: func() (matchMode, *soloMode) {
				mode := &sparringMode{}
				return mode, &mode.soloMode
			},
		},
		{
			name: "expedition",
			build: func() (matchMode, *soloMode) {
				mode := &expeditionMode{phase: expeditionPrep1}
				return mode, &mode.soloMode
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := testHub(t)
			mode, solo := tt.build()
			m := &match{
				a: "trainer", b: dojo.BotTrainer, mode: mode,
				handle: map[string]string{dojo.BotTrainer: "bot"},
			}
			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				for range 1_000 {
					h.mu.Lock()
					solo.lastDecision = dojo.DecisionExplanation{
						PrimaryReason: "test decision",
						ReasonCode:    "test",
					}
					h.mu.Unlock()
				}
			}()
			go func() {
				defer wg.Done()
				<-start
				for range 1_000 {
					mode.pushBattle(h, m)
				}
			}()
			close(start)
			wg.Wait()
		})
	}
}
