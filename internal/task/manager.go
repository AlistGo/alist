package task

import "github.com/xhofe/tache"

type Manager[T tache.Task] interface {
	Add(task T)
	Cancel(id string)
	CancelAll()
	CancelByCondition(condition func(task T) bool)
	GetAll() []T
	GetByID(id string) (T, bool)
	GetByState(state ...tache.State) []T
	GetByCondition(condition func(task T) bool) []T
	Remove(id string)
	RemoveAll()
	RemoveByState(state ...tache.State)
	RemoveByCondition(condition func(task T) bool)
	Retry(id string)
	RetryAllFailed()
}
