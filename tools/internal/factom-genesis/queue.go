package factom

import "sync"

type Queue []interface{}

func (self *Queue) Push(m *sync.Mutex, x interface{}) {
	m.Lock()
	defer m.Unlock()
	*self = append(*self, x)
}

func (self *Queue) Pop(m *sync.Mutex) interface{} {
	m.Lock()
	defer m.Unlock()
	h := *self
	var el interface{}
	l := len(h)
	el, *self = h[0], h[1:l]
	// Or use this instead for a Stack
	// el, *self = h[l-1], h[0:l-1]
	return el
}

func NewQueue() *Queue {
	return &Queue{}
}
