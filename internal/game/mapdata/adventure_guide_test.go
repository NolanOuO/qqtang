package mapdata

import "testing"

func adventureGuideFixture() *AdventureRules {
	return &AdventureRules{
		NPCDropPools: []AdventureNPCDropPool{
			{ID: "npc-herbs", Confidence: "historical-entity-medium", Notes: "按怪物类型配置的候选掉落池"},
			{ID: "npc-boss", Confidence: "historical-boss-medium"},
		},
		WallDropPools: []AdventureWallDropPool{
			{ID: "wall-materials", Confidence: "historical-map-low", Notes: "隐藏墙材料候选池"},
		},
		Maps: []AdventureStageRule{
			{
				MapID: 1601, WallDropPoolID: "wall-materials", WallDropRolls: 2, WallDropChance: 35,
				WallItems: []AdventureNPCDropItem{
					{ItemID: 30007, Quantity: 1, Name: "草药", Kind: "material"},
					{ItemID: 30008, Quantity: 1, Name: "干枯树枝", Kind: "material"},
				},
				NPCDropGroups: []AdventureNPCDropGroup{
					{
						BossID: 41001, BossCount: 3, DropPoolID: "npc-herbs", DropCount: 2, DropChance: 80,
						NormalItems: []AdventureNPCDropItem{
							{ItemID: 30007, Quantity: 1, Weight: 10, Name: "草药", Kind: "material"},
							{ItemID: 30009, Quantity: 1, Weight: 20, Name: "蘑菇", Kind: "material"},
						},
					},
				},
			},
			{
				MapID: 1602,
				NPCDropGroups: []AdventureNPCDropGroup{
					{
						BossID: 41002, BossCount: 1, DropPoolID: "npc-boss", DropCount: 1, DropChance: 95,
						NormalItems: []AdventureNPCDropItem{
							{ItemID: 30007, Quantity: 2, Weight: 5, Name: "草药", Kind: "material"},
						},
					},
				},
			},
		},
	}
}

func TestFindAdventureDropSourcesReturnsConfiguredNPCAndWallSources(t *testing.T) {
	sources, err := adventureGuideFixture().FindAdventureDropSources(30007, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 3 {
		t.Fatalf("source count = %d, want 3: %+v", len(sources), sources)
	}
	if sources[0].MapID != 1601 || sources[0].SourceKind != "npc" || sources[0].NPCID != 41001 ||
		sources[0].NPCInstances != 3 || sources[0].InventoryChancePercent != 80 ||
		sources[0].MaxItemsPerInventory != 2 || sources[0].ItemWeight != 10 ||
		sources[0].TotalCandidateWeight != 30 || sources[0].Confidence != "historical-entity-medium" {
		t.Fatalf("NPC source metadata = %+v", sources[0])
	}
	if sources[1].MapID != 1601 || sources[1].SourceKind != "wall" ||
		sources[1].WallRollCount != 2 || sources[1].WallRollChancePercent != 35 ||
		sources[1].Confidence != "historical-map-low" {
		t.Fatalf("wall source metadata = %+v", sources[1])
	}
	if sources[2].MapID != 1602 || sources[2].SourceKind != "npc" || sources[2].NPCID != 41002 {
		t.Fatalf("second map source = %+v", sources[2])
	}
}

func TestFindAdventureDropSourcesFiltersByNameAndKind(t *testing.T) {
	rules := adventureGuideFixture()
	sources, err := rules.FindAdventureDropSources(0, "草药", "wall")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].SourceKind != "wall" || sources[0].Item.ItemID != 30007 {
		t.Fatalf("wall name search = %+v", sources)
	}

	sources, err = rules.FindAdventureDropSources(30007, "草", "npc")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[0].MapID != 1601 || sources[1].MapID != 1602 {
		t.Fatalf("NPC ID/name search = %+v", sources)
	}

	sources, err = rules.FindAdventureDropSources(30007, "树枝", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatalf("combined filters should use AND semantics, got %+v", sources)
	}
}

func TestFindAdventureDropSourcesRejectsMissingOrInvalidFilters(t *testing.T) {
	if _, err := adventureGuideFixture().FindAdventureDropSources(0, "", ""); err == nil {
		t.Fatal("empty search was accepted")
	}
	if _, err := adventureGuideFixture().FindAdventureDropSources(30007, "", "pet"); err == nil {
		t.Fatal("unknown source kind was accepted")
	}
	if _, err := (*AdventureRules)(nil).FindAdventureDropSources(30007, "", ""); err == nil {
		t.Fatal("nil rules were accepted")
	}
}
