package walletstatedb

import "time"

type Challenge struct {
	Challenge string    `json:"challenge"`
	Hash      string    `json:"hash"`
	Status    string    `json:"status"` // "unused", "used", "expired"
	Npub      string    `json:"npub"`
	CreatedAt time.Time `json:"created_at"`
	UsedAt    time.Time `json:"used_at,omitempty"`
	ExpiredAt time.Time `json:"expired_at,omitempty"`
}

type Address struct {
	Index       uint
	Address     string
	Status      string
	AllocatedAt *time.Time
	UsedAt      *time.Time
	BlockHeight *uint32
}
