// Package ui models the Google Workspace card API described at
// https://developers.google.com/workspace/add-ons/reference/rpc/google.apps.card.v1
package ui

// Top-level object passed from addon to Google Workspaces.
// See https://developers.google.com/workspace/add-ons/reference/rpc/google.apps.card.v1#submitformresponse
type Response struct {
	RenderActions *RenderActions `json:"renderActions"`
	StateChanged  bool           `json:"stateChanged"`
}

type RenderActions struct {
	Action        *Action        `json:"action"`
	HostAppAction *HostAppAction `json:"hostAppAction"`
}

type Action struct {
	Navigations  []*Navigation `json:"navigations"`
	Link         *OpenLink     `json:"link"`
	Notification *Notification `json:"notification"`
}

// not implemented
type Navigation struct{}

type OpenLink struct {
	URL     string  `json:"url"`
	OpenAs  *string `json:"openAs"`  // Can be "FULL_SIZE" or "OVERLAY"
	OnClose *string `json:"onClose"` // Can be "NOTHING" or "RELOAD"
}

// not implemented
type HostAppAction struct{}

type Notification struct{}
