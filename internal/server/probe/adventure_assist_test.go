package probe

import (
	"io"
	"path/filepath"
	"testing"

	"qqtang/internal/game/mapdata"
	"qqtang/internal/game/match"
)

func loadAdventureAssistTestRules(t *testing.T) *mapdata.AdventureRules {
	t.Helper()
	rules, err := mapdata.LoadAdventureRules(filepath.Join("..", "..", "..", "configs", "adventure-rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func newAdventureAssistTestServer(rules *mapdata.AdventureRules) *Server {
	server := &Server{adventureRules: rules, battles: make(map[uint32]*match.AdventureBattle), logWriter: io.Discard}
	server.config.AdventureAssist.DisableVisualDeaths = true
	return server
}

func TestAdventureAssistAttemptClearsWholeStage(t *testing.T) {
	rules := loadAdventureAssistTestRules(t)
	stage, ok := rules.Stage(1601)
	if !ok {
		t.Fatal("test rules have no stage 1601")
	}
	server := newAdventureAssistTestServer(rules)
	participants := []match.AdventureParticipant{{PlayerID: 1, TeamID: 1}}
	if err := server.replaceAdventureBattleWithStage(9, 1601, 1, stage.ExpectedNPCDeaths, 1, participants); err != nil {
		t.Fatal(err)
	}
	cleared, completed, err := server.runAdventureAssistAttempt(1, 9, 1601, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("assist attempt did not complete the stage")
	}
	if uint32(cleared) != stage.ExpectedNPCDeaths {
		t.Fatalf("cleared = %d, want %d", cleared, stage.ExpectedNPCDeaths)
	}
	battle, err := server.adventureBattle(9)
	if err != nil {
		t.Fatal(err)
	}
	if battle.MapID() != 1601 || !battle.StageCompleted() {
		t.Fatalf("battle map=%d completed=%t", battle.MapID(), battle.StageCompleted())
	}
	statistics, err := battle.PlayerStatistics(1)
	if err != nil {
		t.Fatal(err)
	}
	if statistics.KillCount != stage.ExpectedNPCDeaths {
		t.Fatalf("kill count = %d, want %d", statistics.KillCount, stage.ExpectedNPCDeaths)
	}
	var wantCourage uint32
	for _, group := range stage.NPCDropGroups {
		for instance := byte(0); instance < group.BossCount; instance++ {
			wantCourage += group.Courage()
		}
	}
	if statistics.KillScore != wantCourage {
		t.Fatalf("kill score = %d, want %d", statistics.KillScore, wantCourage)
	}
	// Without a route catalog the cleared stage is treated as final: victory
	// locks immediately and waits for the normal loot-grace settlement.
	if !battle.FinalVictoryPending() || battle.Concluded() {
		t.Fatalf("pending=%t concluded=%t, want pending victory", battle.FinalVictoryPending(), battle.Concluded())
	}
}

func TestAdventureAssistAttemptSkipsNaturallyKilledInstances(t *testing.T) {
	rules := loadAdventureAssistTestRules(t)
	stage, ok := rules.Stage(1601)
	if !ok {
		t.Fatal("test rules have no stage 1601")
	}
	server := newAdventureAssistTestServer(rules)
	participants := []match.AdventureParticipant{{PlayerID: 1, TeamID: 1}}
	if err := server.replaceAdventureBattleWithStage(11, 1601, 1, stage.ExpectedNPCDeaths, 1, participants); err != nil {
		t.Fatal(err)
	}
	battle, err := server.adventureBattle(11)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := battle.RecordNPCDeath(7); err != nil {
		t.Fatal(err)
	}
	cleared, completed, err := server.runAdventureAssistAttempt(1, 11, 1601, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("assist attempt did not complete the stage")
	}
	if uint32(cleared) != stage.ExpectedNPCDeaths-1 {
		t.Fatalf("cleared = %d, want %d", cleared, stage.ExpectedNPCDeaths-1)
	}
	statistics, err := battle.PlayerStatistics(1)
	if err != nil {
		t.Fatal(err)
	}
	if statistics.KillCount != stage.ExpectedNPCDeaths {
		t.Fatalf("kill count = %d, want %d", statistics.KillCount, stage.ExpectedNPCDeaths)
	}
}

func TestAdventureAssistScheduleSupersedesAndCancels(t *testing.T) {
	rules := loadAdventureAssistTestRules(t)
	server := newAdventureAssistTestServer(rules)
	server.assistEnabled.Store(true)
	// Long delay: this test covers bookkeeping only and must not wait for a
	// timer fire.
	server.config.AdventureAssist.DelayMS = 30000
	server.scheduleAdventureAssist(1, 9, 1601)
	if got := server.assistPendingCount(); got != 1 {
		t.Fatalf("pending = %d, want 1", got)
	}
	// Rescheduling the same match supersedes the previous timer instead of
	// stacking a second clear.
	server.scheduleAdventureAssist(1, 9, 1601)
	if got := server.assistPendingCount(); got != 1 {
		t.Fatalf("pending after reschedule = %d, want 1", got)
	}
	server.cancelAdventureAssist(9)
	if got := server.assistPendingCount(); got != 0 {
		t.Fatalf("pending after cancel = %d, want 0", got)
	}
	// Unknown stages and a disabled flag never arm a timer.
	server.scheduleAdventureAssist(1, 9, 999999)
	if got := server.assistPendingCount(); got != 0 {
		t.Fatalf("pending for unknown map = %d, want 0", got)
	}
	server.assistEnabled.Store(false)
	server.scheduleAdventureAssist(1, 9, 1601)
	if got := server.assistPendingCount(); got != 0 {
		t.Fatalf("pending while disabled = %d, want 0", got)
	}
}

func TestAdventureAssistToggleClearsPending(t *testing.T) {
	rules := loadAdventureAssistTestRules(t)
	server := newAdventureAssistTestServer(rules)
	server.assistEnabled.Store(true)
	server.config.AdventureAssist.DelayMS = 30000
	server.scheduleAdventureAssist(1, 9, 1601)
	if got := server.assistPendingCount(); got != 1 {
		t.Fatalf("pending = %d, want 1", got)
	}
	status := server.setAdventureAssistEnabledForGM()(false)
	if status.Enabled || status.Pending != 0 {
		t.Fatalf("status after disable = %+v", status)
	}
	if server.assistEnabled.Load() {
		t.Fatal("assist still enabled after GM disable")
	}
	status = server.setAdventureAssistEnabledForGM()(true)
	if !status.Enabled {
		t.Fatalf("status after enable = %+v", status)
	}
}

func TestAdventureAssistTriggerWithoutMatches(t *testing.T) {
	server := &Server{logWriter: io.Discard}
	if got := server.triggerAdventureAssistNow(); got != 0 {
		t.Fatalf("triggered = %d, want 0", got)
	}
}

func TestAdventureAssistRemoveBattleCancelsPending(t *testing.T) {
	rules := loadAdventureAssistTestRules(t)
	server := newAdventureAssistTestServer(rules)
	server.assistEnabled.Store(true)
	server.config.AdventureAssist.DelayMS = 30000
	server.scheduleAdventureAssist(1, 9, 1601)
	if got := server.assistPendingCount(); got != 1 {
		t.Fatalf("pending = %d, want 1", got)
	}
	server.removeAdventureBattle(9)
	if got := server.assistPendingCount(); got != 0 {
		t.Fatalf("pending after battle removal = %d, want 0", got)
	}
}
