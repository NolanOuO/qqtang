package gm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"qqtang/internal/game/mapdata"
)

func TestAdventureDropSourcesEndpointReturnsConfiguredSources(t *testing.T) {
	rules := &mapdata.AdventureRules{
		NPCDropPools: []mapdata.AdventureNPCDropPool{
			{ID: "npc-herbs", Confidence: "historical-entity-medium", Notes: "怪物候选掉落池"},
		},
		WallDropPools: []mapdata.AdventureWallDropPool{
			{ID: "wall-materials", Confidence: "historical-map-low"},
		},
		Maps: []mapdata.AdventureStageRule{
			{
				MapID: 1601, WallDropPoolID: "wall-materials", WallDropRolls: 2, WallDropChance: 35,
				WallItems: []mapdata.AdventureNPCDropItem{{ItemID: 30007, Quantity: 1, Name: "草药", Kind: "material"}},
				NPCDropGroups: []mapdata.AdventureNPCDropGroup{{
					BossID: 41001, BossCount: 3, DropPoolID: "npc-herbs", DropCount: 2, DropChance: 80,
					NormalItems: []mapdata.AdventureNPCDropItem{{ItemID: 30007, Quantity: 1, Weight: 10, Name: "草药", Kind: "material"}},
				}},
			},
		},
	}
	handler := (&Server{adventureRules: rules}).Handler()

	request := httptest.NewRequest(http.MethodGet, "/gm/api/adventure/drop-sources?q=%E8%8D%89%E8%8D%AF&kind=npc", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		ResultCount int                         `json:"result_count"`
		Sources     []mapdata.AdventureDropSource `json:"sources"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ResultCount != 1 || len(result.Sources) != 1 {
		t.Fatalf("result = %+v", result)
	}
	source := result.Sources[0]
	if source.MapID != 1601 || source.NPCID != 41001 || source.NPCInstances != 3 ||
		source.InventoryChancePercent != 80 || source.Item.ItemID != 30007 {
		t.Fatalf("source = %+v", source)
	}
}

func TestAdventureDropSourcesEndpointAcceptsNumericSearchBoxAndRejectsBadRequests(t *testing.T) {
	rules := &mapdata.AdventureRules{
		NPCDropPools: []mapdata.AdventureNPCDropPool{{ID: "npc-herbs", Confidence: "medium"}},
		Maps: []mapdata.AdventureStageRule{{
			MapID: 1601,
			NPCDropGroups: []mapdata.AdventureNPCDropGroup{{
				BossID: 41001, BossCount: 1, DropPoolID: "npc-herbs", DropCount: 1, DropChance: 100,
				NormalItems: []mapdata.AdventureNPCDropItem{{ItemID: 30007, Quantity: 1, Weight: 1, Name: "草药"}},
			}},
		}},
	}
	handler := (&Server{adventureRules: rules}).Handler()

	for _, test := range []struct {
		path string
		want int
	}{
		{path: "/gm/api/adventure/drop-sources?q=30007", want: http.StatusOK},
		{path: "/gm/api/adventure/drop-sources", want: http.StatusBadRequest},
		{path: "/gm/api/adventure/drop-sources?q=30007&kind=pet", want: http.StatusBadRequest},
		{path: "/gm/api/adventure/drop-sources?item_id=0", want: http.StatusBadRequest},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Errorf("%s status = %d, want %d: %s", test.path, response.Code, test.want, response.Body.String())
		}
	}
}

func TestAdventureDropSourcesEndpointRequiresGETAndLoadedRules(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/gm/api/adventure/drop-sources?q=30007", nil)
	response := httptest.NewRecorder()
	(&Server{}).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}

	request = httptest.NewRequest(http.MethodGet, "/gm/api/adventure/drop-sources?q=30007", nil)
	response = httptest.NewRecorder()
	(&Server{}).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing rules status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}
