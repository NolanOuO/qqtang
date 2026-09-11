package probe

import (
	"fmt"
	"strconv"
	"time"

	"qqtang/internal/game/adventureassist"
	gmserver "qqtang/internal/server/gm"
	"qqtang/internal/protocol/game"
)

// adventureAssistKillStaggerMS separates synthetic per-instance ClientTime
// values so drops/loot keyed by (item, time, position) stay distinct.
const adventureAssistKillStaggerMS = 50

// adventureAssistBaseClientTime derives a plausible scene-clock value for
// synthetic deaths from the elapsed schedule delay. ClientTime is
// informational (loot identity, GameOver.Time); deriving it from the delay
// keeps values in scene-elapsed range instead of wrapping wall-clock millis.
func adventureAssistBaseClientTime(scheduledAt time.Time, delay time.Duration) uint32 {
	elapsed := time.Since(scheduledAt)
	if elapsed < 0 {
		elapsed = 0
	}
	total := elapsed + delay
	if total < 0 {
		total = 0
	}
	return uint32(total.Milliseconds() & 0xFFFFFFFF)
}

// scheduleAdventureAssist arms one single-flight instant clear for the given
// stable match GameID. It is called after GAME_BEGIN (match start) and after
// every GAME_NEXTMAP advance. Rescheduling supersedes any pending timer for
// the same match, and the timer revalidates battle, map, and GM state before
// killing anything.
func (server *Server) scheduleAdventureAssist(roomID uint16, gameID, mapID uint32) {
	if server == nil || roomID == 0 || gameID == 0 || mapID == 0 {
		return
	}
	if !server.assistEnabled.Load() {
		return
	}
	stage, exists := server.adventureStageRule(mapID)
	if !exists {
		return
	}
	if plan := adventureassist.PlanForStage(stage); plan.IsEmpty() {
		return
	}
	delay := server.config.AdventureAssist.assistDelay()
	scheduledAt := time.Now()
	server.assistMu.Lock()
	if server.assistPending == nil {
		server.assistPending = make(map[uint32]uint64)
	}
	server.assistEpoch++
	epoch := server.assistEpoch
	server.assistPending[gameID] = epoch
	server.assistMu.Unlock()
	server.runAfter(delay, func() {
		server.runAdventureAssist(roomID, gameID, mapID, epoch, scheduledAt, delay)
	})
	server.log(logEvent{
		Level: "info", Event: "adventure_assist_scheduled",
		RoomID: fmt.Sprint(roomID), Result: fmt.Sprintf("game_%d_map_%d_delay_%s", gameID, mapID, delay),
	})
}

// cancelAdventureAssist drops any pending timer for the match. It is called
// when the battle is removed so a late timer cannot touch a recycled GameID.
func (server *Server) cancelAdventureAssist(gameID uint32) {
	if server == nil || gameID == 0 {
		return
	}
	server.assistMu.Lock()
	delete(server.assistPending, gameID)
	server.assistMu.Unlock()
}

func (server *Server) runAdventureAssist(roomID uint16, gameID, mapID uint32, epoch uint64, scheduledAt time.Time, delay time.Duration) {
	if server == nil {
		return
	}
	baseTime := adventureAssistBaseClientTime(scheduledAt, delay)
	err := runRoomActor(server, roomID, "adventure-assist-clear", func() error {
		server.runAdventureAssistOnRoomActor(roomID, gameID, mapID, epoch, baseTime)
		return nil
	})
	if err != nil {
		server.log(logEvent{Level: "error", Event: "adventure_assist_actor_failed", RoomID: fmt.Sprint(roomID), Result: fmt.Sprintf("game_%d_map_%d", gameID, mapID), ErrorContext: err.Error()})
	}
}

func (server *Server) runAdventureAssistOnRoomActor(roomID uint16, gameID, mapID uint32, epoch uint64, baseTime uint32) {
	server.assistMu.Lock()
	pending, scheduled := server.assistPending[gameID]
	if scheduled && pending == epoch {
		delete(server.assistPending, gameID)
	} else {
		server.assistMu.Unlock()
		return
	}
	server.assistMu.Unlock()
	// A GM disable between schedule and fire cancels the clear. Manual
	// trigger uses runAdventureAssistAttempt directly and is unaffected.
	if !server.assistEnabled.Load() {
		server.log(logEvent{Level: "info", Event: "adventure_assist_skipped_disabled", RoomID: fmt.Sprint(roomID), Result: fmt.Sprintf("game_%d_map_%d", gameID, mapID)})
		return
	}
	cleared, newlyCompleted, err := server.runAdventureAssistAttempt(roomID, gameID, mapID, baseTime)
	if err != nil {
		server.log(logEvent{Level: "error", Event: "adventure_assist_failed", RoomID: fmt.Sprint(roomID), Result: fmt.Sprintf("game_%d_map_%d", gameID, mapID), ErrorContext: err.Error()})
		return
	}
	if cleared == 0 {
		return
	}
	result := fmt.Sprintf("game_%d_map_%d_kills_%d", gameID, mapID, cleared)
	if newlyCompleted {
		result += "_stage_completed"
	}
	server.log(logEvent{Level: "info", Event: "adventure_assist_stage_cleared", RoomID: fmt.Sprint(roomID), Result: result})
}

// runAdventureAssistAttempt performs the deterministic kill loop for one
// stage. It must run on the room actor. It returns the number of NPC
// instances cleared and whether this attempt completed the stage.
//
// Kills already reported by clients in the scheduling window are skipped by
// ordinal so courage is never double-credited: plan order is normals-first,
// matching natural play.
func (server *Server) runAdventureAssistAttempt(roomID uint16, gameID, mapID uint32, baseTime uint32) (cleared int, newlyCompleted bool, err error) {
	battle, err := server.adventureBattle(gameID)
	if err != nil {
		return 0, false, nil
	}
	if battle.MapID() != mapID || battle.Concluded() || battle.FinalVictoryPending() || battle.StageCompleted() {
		return 0, false, nil
	}
	stage, exists := server.adventureStageRule(mapID)
	if !exists {
		return 0, false, nil
	}
	plan := adventureassist.PlanForStage(stage)
	if plan.IsEmpty() {
		return 0, false, nil
	}
	recorded, _ := battle.NPCProgress()
	start := int(recorded)
	if start < 0 {
		start = 0
	}
	if start >= len(plan.Kills) {
		return 0, false, nil
	}
	visualDeaths := !server.config.AdventureAssist.DisableVisualDeaths
	lastClientTime := baseTime
	for _, kill := range plan.Kills[start:] {
		clientTime := baseTime + kill.Ordinal*adventureAssistKillStaggerMS
		lastClientTime = clientTime
		if visualDeaths {
			body, marshalErr := game.PlayerDeathEvent{
				PlayerID: kill.BossID, ClientTime: clientTime,
			}.MarshalNetworkBinary()
			if marshalErr != nil {
				return cleared, newlyCompleted, marshalErr
			}
			killBody := body
			killID := kill.BossID
			server.broadcastRoomNotification(roomID, 0,
				fmt.Sprintf("qqt_adventure_assist_npc_death_%d", killID),
				func(recipientPacket []byte) ([]byte, error) {
					return game.BuildLocalGameEventPushForRecipient(recipientPacket, roomID, server.nextGameDataSequence(), game.NotifyPlayerDieEvent, killBody)
				})
		}
		progress, recordErr := battle.RecordNPCDeath(kill.Courage)
		if recordErr != nil {
			return cleared, newlyCompleted, recordErr
		}
		if !progress.Recorded {
			break
		}
		cleared++
		if progress.NewlyCompleted {
			newlyCompleted = true
			break
		}
	}
	if !newlyCompleted {
		return cleared, false, nil
	}
	if server.mapCatalog != nil {
		if _, hasNext := server.mapCatalog.Next(mapID); hasNext {
			return cleared, true, nil
		}
	}
	final, err := battle.BeginFinalStageVictory()
	if err != nil {
		return cleared, true, err
	}
	if final.NewlyConcluded {
		server.scheduleAdventureFinalVictory(adventureFinalVictorySchedule{
			gameID: gameID, roomID: roomID, clientTime: lastClientTime, battle: battle,
		})
	}
	return cleared, true, nil
}

// assistPendingCount reports armed timers for GM status.
func (server *Server) assistPendingCount() int {
	if server == nil {
		return 0
	}
	server.assistMu.Lock()
	defer server.assistMu.Unlock()
	return len(server.assistPending)
}

// triggerAdventureAssistNow clears the current stage of every active
// adventure match immediately, bypassing the schedule delay and the enabled
// flag. It returns the number of matches that had instances cleared.
func (server *Server) triggerAdventureAssistNow() int {
	if server == nil {
		return 0
	}
	type assistTarget struct {
		roomID uint16
		gameID uint32
		mapID  uint32
	}
	server.liveMu.RLock()
	candidates := make([]*connectionSession, 0, len(server.liveSessions))
	for _, session := range server.liveSessions {
		if session != nil && session.liveRoomID.Load() != 0 && session.liveUIN.Load() != 0 {
			candidates = append(candidates, session)
		}
	}
	server.liveMu.RUnlock()
	seen := make(map[uint32]assistTarget)
	for _, session := range candidates {
		session.mu.Lock()
		roomID, gameID, mapID := session.RoomID, session.CurrentGameID, session.CurrentMapID
		session.mu.Unlock()
		if roomID == 0 || gameID == 0 || mapID == 0 {
			continue
		}
		if _, ok := seen[gameID]; !ok {
			seen[gameID] = assistTarget{roomID: roomID, gameID: gameID, mapID: mapID}
		}
	}
	triggered := 0
	for _, target := range seen {
		cleared, _, err := callRoomActor(server, target.roomID, "adventure-assist-trigger", func() (adventureAssistTriggerOutcome, error) {
			// Manual trigger supersedes any armed timer for the match.
			server.assistMu.Lock()
			delete(server.assistPending, target.gameID)
			server.assistMu.Unlock()
			cleared, completed, attemptErr := server.runAdventureAssistAttempt(target.roomID, target.gameID, target.mapID, 0)
			return adventureAssistTriggerOutcome{cleared: cleared, completed: completed}, attemptErr
		})
		if err != nil {
			server.log(logEvent{Level: "error", Event: "adventure_assist_trigger_failed", RoomID: fmt.Sprint(target.roomID), Result: "game_" + strconv.FormatUint(uint64(target.gameID), 10), ErrorContext: err.Error()})
			continue
		}
		if cleared.cleared > 0 {
			triggered++
		}
	}
	return triggered
}

type adventureAssistTriggerOutcome struct {
	cleared   int
	completed bool
}

// adventureAssistStatusForGM exposes the assist state to the GM bridge.
func (server *Server) adventureAssistStatusForGM() gmserver.AdventureAssistStatusFunc {
	return func() gmserver.AdventureAssistStatus {
		return gmserver.AdventureAssistStatus{
			Enabled:             server.assistEnabled.Load(),
			DelayMS:             int(server.config.AdventureAssist.assistDelay().Milliseconds()),
			DisableVisualDeaths: server.config.AdventureAssist.DisableVisualDeaths,
			Pending:             server.assistPendingCount(),
		}
	}
}

// setAdventureAssistEnabledForGM flips the auto-clear flag at runtime without
// touching persisted config. Disabling cancels armed timers logically: timers
// already fired keep their credited kills, timers still pending observe the
// flag and skip.
func (server *Server) setAdventureAssistEnabledForGM() gmserver.AdventureAssistSetEnabledFunc {
	return func(enabled bool) gmserver.AdventureAssistStatus {
		server.assistEnabled.Store(enabled)
		if !enabled {
			server.assistMu.Lock()
			server.assistPending = make(map[uint32]uint64)
			server.assistMu.Unlock()
		}
		server.log(logEvent{Level: "info", Event: "adventure_assist_toggled", Result: fmt.Sprintf("enabled_%t", enabled)})
		return server.adventureAssistStatusForGM()()
	}
}

// triggerAdventureAssistForGM maps the GM manual-trigger endpoint onto the
// immediate room-actor clear.
func (server *Server) triggerAdventureAssistForGM() gmserver.AdventureAssistTriggerFunc {
	return func() int {
		return server.triggerAdventureAssistNow()
	}
}
