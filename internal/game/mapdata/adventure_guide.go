package mapdata

import (
	"fmt"
	"sort"
	"strings"
)

// AdventureDropSource describes one configured source for an item in an
// adventure stage. The chance fields intentionally describe the server's
// inventory-generation roll, not a guaranteed per-kill item drop rate.
type AdventureDropSource struct {
	MapID                      uint32                `json:"map_id"`
	SourceKind                 string                `json:"source_kind"`
	PoolID                     string                `json:"pool_id"`
	NPCID                      uint16                `json:"npc_id,omitempty"`
	NPCInstances               uint8                 `json:"npc_instances,omitempty"`
	InventoryChancePercent     uint8                 `json:"inventory_chance_percent,omitempty"`
	MaxItemsPerInventory       uint8                 `json:"max_items_per_inventory,omitempty"`
	ItemWeight                 uint16                `json:"item_weight,omitempty"`
	TotalCandidateWeight       uint32                `json:"total_candidate_weight,omitempty"`
	WallRollCount              uint8                 `json:"wall_roll_count,omitempty"`
	WallRollChancePercent      uint8                 `json:"wall_roll_chance_percent,omitempty"`
	Item                       AdventureNPCDropItem  `json:"item"`
	Confidence                 string                `json:"confidence,omitempty"`
	Notes                      string                `json:"notes,omitempty"`
}

// FindAdventureDropSources finds configured NPC and hidden-wall sources for
// an item. A non-zero itemID filters by exact ID; nameQuery filters by a
// case-insensitive substring of the configured item name. When both are
// supplied, both filters must match. sourceKind is "", "npc", or "wall".
//
// For NPCs, InventoryChancePercent is the chance that the server creates the
// candidate inventory for that NPC type; ItemWeight is the selected item's
// weight within that inventory. The client can still choose a subset, so this
// is not an item-specific per-kill probability.
//
// For walls, WallRollChancePercent applies independently to each configured
// wall roll. Successful rolls choose from the stage's material pool; it is
// not a guarantee that this particular item appears.
func (rules *AdventureRules) FindAdventureDropSources(itemID uint32, nameQuery, sourceKind string) ([]AdventureDropSource, error) {
	if rules == nil {
		return nil, fmt.Errorf("adventure rules are unavailable")
	}
	nameQuery = strings.ToLower(strings.TrimSpace(nameQuery))
	sourceKind = strings.ToLower(strings.TrimSpace(sourceKind))
	if itemID == 0 && nameQuery == "" {
		return nil, fmt.Errorf("an item ID or name query is required")
	}
	if sourceKind != "" && sourceKind != "npc" && sourceKind != "wall" {
		return nil, fmt.Errorf("unknown adventure drop source kind %q", sourceKind)
	}

	npcPools := make(map[string]AdventureNPCDropPool, len(rules.NPCDropPools))
	for _, pool := range rules.NPCDropPools {
		npcPools[pool.ID] = pool
	}
	wallPools := make(map[string]AdventureWallDropPool, len(rules.WallDropPools))
	for _, pool := range rules.WallDropPools {
		wallPools[pool.ID] = pool
	}

	matches := func(item AdventureNPCDropItem) bool {
		if itemID != 0 && item.ItemID != itemID {
			return false
		}
		return nameQuery == "" || strings.Contains(strings.ToLower(item.Name), nameQuery)
	}

	sources := make([]AdventureDropSource, 0)
	for _, stage := range rules.Maps {
		if sourceKind == "" || sourceKind == "npc" {
			for _, group := range stage.NPCDropGroups {
				var totalWeight uint32
				for _, candidate := range group.NormalItems {
					totalWeight += uint32(candidate.Weight)
				}
				pool := npcPools[group.DropPoolID]
				for _, item := range group.NormalItems {
					if !matches(item) {
						continue
					}
					sources = append(sources, AdventureDropSource{
						MapID: stage.MapID, SourceKind: "npc", PoolID: group.DropPoolID,
						NPCID: group.BossID, NPCInstances: group.BossCount,
						InventoryChancePercent: group.DropChance,
						MaxItemsPerInventory: group.DropCount,
						ItemWeight: item.Weight, TotalCandidateWeight: totalWeight,
						Item: item, Confidence: pool.Confidence, Notes: pool.Notes,
					})
				}
			}
		}
		if sourceKind == "" || sourceKind == "wall" {
			pool := wallPools[stage.WallDropPoolID]
			for _, item := range stage.WallItems {
				if !matches(item) {
					continue
				}
				sources = append(sources, AdventureDropSource{
					MapID: stage.MapID, SourceKind: "wall", PoolID: stage.WallDropPoolID,
					WallRollCount: stage.WallDropRolls,
					WallRollChancePercent: stage.WallDropChance,
					Item: item, Confidence: pool.Confidence, Notes: pool.Notes,
				})
			}
		}
	}

	sort.Slice(sources, func(i, j int) bool {
		left, right := sources[i], sources[j]
		if left.MapID != right.MapID {
			return left.MapID < right.MapID
		}
		if left.SourceKind != right.SourceKind {
			return left.SourceKind < right.SourceKind
		}
		if left.NPCID != right.NPCID {
			return left.NPCID < right.NPCID
		}
		return left.Item.ItemID < right.Item.ItemID
	})
	return sources, nil
}
