package gm

import (
	"fmt"
	"net/http"
)

// AdventureAssistStatus is the GM projection of the local-server adventure
// instant-clear assistant. DelayMS is the configured schedule delay in
// milliseconds; Pending counts armed auto-clear timers.
type AdventureAssistStatus struct {
	Enabled             bool `json:"enabled"`
	DelayMS             int  `json:"delay_ms"`
	DisableVisualDeaths bool `json:"disable_visual_deaths"`
	Pending             int  `json:"pending"`
}

// AdventureAssistStatusFunc reads the current assistant state.
type AdventureAssistStatusFunc func() AdventureAssistStatus

// AdventureAssistSetEnabledFunc flips the auto-clear flag at runtime.
type AdventureAssistSetEnabledFunc func(enabled bool) AdventureAssistStatus

// AdventureAssistTriggerFunc clears the current stage of every active
// adventure match immediately and returns how many matches were cleared.
type AdventureAssistTriggerFunc func() int

type adventureAssistControl struct {
	status     AdventureAssistStatusFunc
	setEnabled AdventureAssistSetEnabledFunc
	trigger    AdventureAssistTriggerFunc
}

// WithAdventureAssistControl installs the probe-owned instant-clear bridge.
// The GM package never imports the game server, so the probe layer supplies
// these callbacks the same way it supplies the online check.
func WithAdventureAssistControl(status AdventureAssistStatusFunc, setEnabled AdventureAssistSetEnabledFunc, trigger AdventureAssistTriggerFunc) Option {
	return func(server *Server) error {
		if status == nil || setEnabled == nil || trigger == nil {
			return fmt.Errorf("GM adventure assist control is nil")
		}
		server.adventureAssist = &adventureAssistControl{status: status, setEnabled: setEnabled, trigger: trigger}
		return nil
	}
}

func (server *Server) handleAdventureAssist(writer http.ResponseWriter, request *http.Request) {
	control := server.adventureAssist
	if control == nil {
		writeError(writer, http.StatusServiceUnavailable, "探险秒杀助手未接入")
		return
	}
	switch request.Method {
	case http.MethodGet:
		writeJSON(writer, http.StatusOK, control.status())
	case http.MethodPost:
		var input struct {
			Enabled *bool `json:"enabled"`
		}
		if !decodeJSONBody(writer, request, &input) {
			return
		}
		if input.Enabled == nil {
			writeError(writer, http.StatusBadRequest, "缺少 enabled 字段")
			return
		}
		writeJSON(writer, http.StatusOK, control.setEnabled(*input.Enabled))
	default:
		writeMethodNotAllowed(writer, http.MethodGet, http.MethodPost)
	}
}

func (server *Server) handleAdventureAssistTrigger(writer http.ResponseWriter, request *http.Request) {
	control := server.adventureAssist
	if control == nil {
		writeError(writer, http.StatusServiceUnavailable, "探险秒杀助手未接入")
		return
	}
	if request.Method != http.MethodPost {
		writeMethodNotAllowed(writer, http.MethodPost)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"triggered": control.trigger()})
}
