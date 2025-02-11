package restic

import (
	"sync"
)

var messagePool *statusMessagePool

type statusMessagePool struct {
	pool sync.Pool
}

func init() {
	messagePool = NewResticMessagePool()
}

func NewResticMessagePool() *statusMessagePool {
	return &statusMessagePool{
		pool: sync.Pool{
			New: func() any {
				obj := new(StatusUpdate)
				return obj
			},
		},
	}
}

var count int

func (r *statusMessagePool) Get() *StatusUpdate {
	if obj := r.pool.Get(); obj != nil {
		return obj.(*StatusUpdate)
	}
	var obj = new(StatusUpdate)
	count = count + 1

	r.Put(obj)
	return obj
}

func (r *statusMessagePool) Put(obj *StatusUpdate) {
	r.pool.Put(obj)
}
