package factom

import "sync"

type Queue struct {
	m sync.Mutex
	q []interface{}
}

func (q *Queue) Push(x interface{}) {
	q.m.Lock()
	defer q.m.Unlock()
	q.q = append(q.q, x)
}

func (q *Queue) Pop() interface{} {
	q.m.Lock()
	defer q.m.Unlock()
	h := q.q
	var el interface{}
	l := len(h)
	el, q.q = h[0], h[1:l]
	// Or use this instead for a Stack
	// el, *self = h[l-1], h[0:l-1]
	return el
}

func NewQueue() *Queue {
	return &Queue{}
}
