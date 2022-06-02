package factom

import "sync"

type Queue struct {
	m sync.Mutex
	q []interface{}
}

func (self *Queue) Push(x interface{}) {
	self.m.Lock()
	defer self.m.Unlock()
	*&self.q = append(*&self.q, x)
}

func (self *Queue) Pop() interface{} {
	self.m.Lock()
	defer self.m.Unlock()
	h := *&self.q
	var el interface{}
	l := len(h)
	el, *&self.q = h[0], h[1:l]
	// Or use this instead for a Stack
	// el, *self = h[l-1], h[0:l-1]
	return el
}

func NewQueue() *Queue {
	return &Queue{}
}
