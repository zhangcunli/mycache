package mycache

import (
	_ "sync"
	"time"
)

type Clock interface {
	Now() int64
	ExpireAt(int64) int64
}

type RealClock struct{}

func NewRealClock() Clock {
	return RealClock{}
}

func (rc RealClock) Now() int64 {
	t := time.Now().Unix()
	return t
}

func (rc RealClock) ExpireAt(expireSeconds int64) int64 {
	expireAt := int64(0)
	if expireSeconds > 0 {
		expireAt = rc.Now() + expireSeconds
	}
	return expireAt
}
