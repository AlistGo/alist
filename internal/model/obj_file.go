package model

import "time"

type ObjFile struct {
	Id          string    `json:"id"`
	UserId      uint      `json:"user_id"`
	Path        string    `json:"path"`
	Name        string    `json:"name"`
	Size        int64     `json:"size"`
	IsDir       bool      `json:"is_dir"`
	Modified    time.Time `json:"modified"`
	Created     time.Time `json:"created"`
	Sign        string    `json:"sign"`
	Thumb       string    `json:"thumb"`
	Type        int       `json:"type"`
	HashInfoStr string    `json:"hashinfo"`
}
