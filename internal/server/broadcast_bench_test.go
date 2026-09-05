package server

import (
	"fmt"
	"testing"
)

// BenchmarkBroadcastDojo32 measures one full broadcast pass to a packed
// 32-seat Dojo: the per-tick work a single walker triggers in a full room.
// Every trainer is attached with a no-op sender so packet assembly (shared
// presence snapshot, per-recipient Others slices, queue membership) is
// included; only channel writes are excluded.
func BenchmarkBroadcastDojo32(b *testing.B) {
	h := testHub(b)
	for i := 1; i <= 32; i++ {
		onboardTrainer(b, h, fmt.Sprintf("trainer-%02d", i), "rootkit")
	}
	for i := 1; i <= 32; i++ {
		h.Attach(fmt.Sprintf("trainer-%02d", i), func(any) {}, func() {})
	}
	id, ok := h.dojoOf["trainer-01"]
	if !ok {
		b.Fatal("trainers not seated")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out outbox
		h.broadcastDojoLocked(&out, id)
	}
}
