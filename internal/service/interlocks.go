package service

import (
	"sort"
	"time"
)

type Interlock struct {
	Name      string    `json:"name"`
	Satisfied bool      `json:"satisfied"`
	Detail    string    `json:"detail"`
	CheckedAt time.Time `json:"checked_at"`
}

func (runtime *Runtime) Interlocks() []Interlock {
	now := time.Now().UTC()
	_, _, _, threaded, _ := runtime.web.Snapshot()
	status, _ := runtime.dryerControl.Status()
	ready, _, _, fault, _ := runtime.folder.Snapshot()
	inkColor := runtime.inkControl.Active()
	inkReady := runtime.inkControl.Ready()
	inkDetail := "active ink circuit is open"
	if !inkReady {
		if inkColor == "" {
			inkDetail = "no active ink color after failed color switch"
		} else {
			inkDetail = "active ink color " + inkColor + " circuit is not open"
		}
	}
	values := []Interlock{
		{Name: "web_threaded", Satisfied: threaded, Detail: "paper web is fenced and threaded", CheckedAt: now},
		{Name: "dryer_ready", Satisfied: status == "ready", Detail: "dryer damper and profile startup are complete", CheckedAt: now},
		{Name: "folder_ready", Satisfied: ready, Detail: fault, CheckedAt: now},
		{Name: "ink_route_ready", Satisfied: inkReady, Detail: inkDetail, CheckedAt: now},
		{Name: "press_stopped_for_splice", Satisfied: runtime.pressControl.CanSplice(), Detail: "current stop epoch has zero-speed proof", CheckedAt: now},
	}
	sort.Slice(values, func(left, right int) bool { return values[left].Name < values[right].Name })
	return values
}
