package servicediscovery

import "time"

// EventType is the discovery event type.
type EventType string

const (
	EventJoin      EventType = "join"
	EventLeave     EventType = "leave"
	EventKeepAlive EventType = "keep-alive"
)

// WireMessage is the JSON message published on the discovery topic.
type WireMessage struct {
	Event           EventType `json:"event"`
	ID              int64     `json:"id"`
	Key             string    `json:"key"`
	PrivateEndPoint string    `json:"privateEndPoint"`
	PublicEndPoint  string    `json:"publicEndPoint"`
	Type            string    `json:"type"`
	Version         string    `json:"version"`
}

// Instance is the in-memory representation of a discovered service instance.
type Instance struct {
	ID              int64
	Key             string
	Type            string
	Version         string
	PrivateEndPoint string
	PublicEndPoint  string
	LastSeenUTC     time.Time
}
