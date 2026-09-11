package adventureassist

import (
	"sort"

	"qqtang/internal/game/mapdata"
)

// PlannedKill is one server-driven NPC instance kill performed by the
// adventure instant-clear assistant. BossID identifies the battlefield NPC
// base type; the same BossID appears BossCount times when a stage contains
// repeated monsters. Courage is the cooperative score awarded for that
// instance, matching the value used by the normal NOTIFY_PLAYER_DIE path.
type PlannedKill struct {
	BossID  uint16
	Courage uint32
	IsBoss  bool
	Ordinal uint32
}

// StagePlan is the deterministic kill order for one adventure stage.
// Normal monsters come first (BossID ascending), Boss instances last, so the
// client observes minions dying before the Boss death cinematic.
type StagePlan struct {
	MapID          uint32
	ExpectedDeaths uint32
	Kills          []PlannedKill
}

// PlanForStage expands the stage's NPC drop groups into per-instance kills.
// It returns an empty plan when the stage has no NPC groups.
func PlanForStage(stage mapdata.AdventureStageRule) StagePlan {
	plan := StagePlan{MapID: stage.MapID, ExpectedDeaths: stage.ExpectedNPCDeaths}
	type groupView struct {
		bossID  uint16
		count   int
		courage uint32
		isBoss  bool
	}
	groups := make([]groupView, 0, len(stage.NPCDropGroups))
	for _, group := range stage.NPCDropGroups {
		if group.BossID == 0 || group.BossCount == 0 {
			continue
		}
		isBoss := false
		for _, profile := range group.CombatProfiles {
			if profile.IsBoss {
				isBoss = true
				break
			}
		}
		groups = append(groups, groupView{
			bossID:  group.BossID,
			count:   int(group.BossCount),
			courage: group.Courage(),
			isBoss:  isBoss,
		})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].isBoss != groups[j].isBoss {
			return !groups[i].isBoss && groups[j].isBoss
		}
		return groups[i].bossID < groups[j].bossID
	})
	ordinal := uint32(0)
	for _, group := range groups {
		for instance := 0; instance < group.count; instance++ {
			ordinal++
			plan.Kills = append(plan.Kills, PlannedKill{
				BossID:  group.bossID,
				Courage: group.courage,
				IsBoss:  group.isBoss,
				Ordinal: ordinal,
			})
		}
	}
	return plan
}

// IsEmpty reports whether the plan contains no kills.
func (plan StagePlan) IsEmpty() bool {
	return len(plan.Kills) == 0
}

// BossKills returns only the Boss instances in the plan.
func (plan StagePlan) BossKills() []PlannedKill {
	var bosses []PlannedKill
	for _, kill := range plan.Kills {
		if kill.IsBoss {
			bosses = append(bosses, kill)
		}
	}
	return bosses
}

// TotalCourage is the sum of per-instance courage across the plan.
func (plan StagePlan) TotalCourage() uint64 {
	var total uint64
	for _, kill := range plan.Kills {
		total += uint64(kill.Courage)
	}
	return total
}
