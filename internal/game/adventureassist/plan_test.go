package adventureassist

import (
	"testing"

	"qqtang/internal/game/mapdata"
)

func combatFixture(count byte, isBoss bool) mapdata.AdventureNPCCombatProfile {
	return mapdata.AdventureNPCCombatProfile{
		Count: count,
		AdventureNPCCombat: mapdata.AdventureNPCCombat{
			Health: 100, Damage: 10, Defense: 0, Speed: 10, IsBoss: isBoss,
		},
	}
}

func stageFixture() mapdata.AdventureStageRule {
	return mapdata.AdventureStageRule{
		MapID:             1601,
		ExpectedNPCDeaths: 4,
		NPCDropGroups: []mapdata.AdventureNPCDropGroup{
			{
				BossID:         12,
				BossCount:      2,
				CombatProfiles: []mapdata.AdventureNPCCombatProfile{combatFixture(2, false)},
			},
			{
				BossID:         3,
				BossCount:      1,
				CombatProfiles: []mapdata.AdventureNPCCombatProfile{combatFixture(1, false)},
			},
			{
				BossID:         99,
				BossCount:      1,
				CombatProfiles: []mapdata.AdventureNPCCombatProfile{combatFixture(1, true)},
			},
		},
	}
}

func TestPlanForStageOrdersNormalsBeforeBoss(t *testing.T) {
	stage := stageFixture()
	plan := PlanForStage(stage)
	if plan.IsEmpty() {
		t.Fatal("expected a non-empty plan")
	}
	if got, want := len(plan.Kills), 4; got != want {
		t.Fatalf("kills = %d, want %d", got, want)
	}
	if plan.MapID != stage.MapID || plan.ExpectedDeaths != stage.ExpectedNPCDeaths {
		t.Fatalf("plan header = map %d expected %d", plan.MapID, plan.ExpectedDeaths)
	}
	wantOrder := []uint16{3, 12, 12, 99}
	for index, bossID := range wantOrder {
		kill := plan.Kills[index]
		if kill.BossID != bossID {
			t.Fatalf("kill %d BossID = %d, want %d", index, kill.BossID, bossID)
		}
		if kill.Ordinal != uint32(index+1) {
			t.Fatalf("kill %d ordinal = %d, want %d", index, kill.Ordinal, index+1)
		}
		group, ok := stage.NPCDropGroup(bossID)
		if !ok {
			t.Fatalf("fixture has no group for BossID %d", bossID)
		}
		if kill.Courage != group.Courage() {
			t.Fatalf("kill %d courage = %d, want group courage %d", index, kill.Courage, group.Courage())
		}
	}
	if plan.Kills[3].IsBoss != true {
		t.Fatalf("last kill IsBoss = false, want the boss last")
	}
	for _, kill := range plan.Kills[:3] {
		if kill.IsBoss {
			t.Fatalf("normal kill %+v flagged as boss", kill)
		}
	}
	if bosses := plan.BossKills(); len(bosses) != 1 || bosses[0].BossID != 99 {
		t.Fatalf("boss kills = %+v", bosses)
	}
	var wantTotal uint64
	for _, kill := range plan.Kills {
		wantTotal += uint64(kill.Courage)
	}
	if total := plan.TotalCourage(); total != wantTotal || total == 0 {
		t.Fatalf("total courage = %d, want %d", total, wantTotal)
	}
}

func TestPlanForStageSkipsEmptyGroups(t *testing.T) {
	stage := mapdata.AdventureStageRule{MapID: 1602, NPCDropGroups: nil}
	if plan := PlanForStage(stage); !plan.IsEmpty() {
		t.Fatalf("empty stage plan = %+v", plan.Kills)
	}
	stage.NPCDropGroups = []mapdata.AdventureNPCDropGroup{{BossID: 0, BossCount: 3}, {BossID: 7, BossCount: 0}}
	if plan := PlanForStage(stage); !plan.IsEmpty() {
		t.Fatalf("zero-ID/zero-count stage plan = %+v", plan.Kills)
	}
}
