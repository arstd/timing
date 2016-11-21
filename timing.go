package timing

import (
	"container/heap"
	"fmt"
	"sync/atomic"
	"time"
)

type HandlerFunc func(items ...*Item)

var (
	// PersistFunc 落地定时项，以便系统重启后，恢复定时项
	PersistFunc HandlerFunc = func(items ...*Item) {}

	// RemoveFunc 时间到了，从落地库中移除定时项
	RemoveFunc HandlerFunc = func(items ...*Item) {}

	// RemindFunc 时间到了，提醒，处理定时项
	RemindFunc HandlerFunc = func(items ...*Item) {
		fmt.Printf("default remind: %#v\n", items)
	}
)

var (
	stage  = make(chan *Item)
	inited = make(chan struct{})
)

var next = func() func() uint32 {
	var id uint32
	return func() uint32 {
		return atomic.AddUint32(&id, 1)
	}
}()

// Init 初始化定时器，并启动
func Init(items ...*Item) {
	q := make(Queue, len(items), 1024)
	for i, item := range items {
		item.Id = next()
		q[i] = item
		PersistFunc(item)
	}
	go start(q)
}

// Add 用来添加定时项并设置ID，必须先 Init，否则 Add 会一直等待 Init
func Add(items ...*Item) {
	<-inited

	for _, item := range items {
		item.Id = next()
		stage <- item
	}
}

func start(q Queue) {
	heap.Init(&q)

	var min *Item
	var timer = time.NewTimer(24 * time.Hour)

	if len(q) > 0 {
		min = heap.Pop(&q).(*Item)
		timer = time.NewTimer(time.Unix(int64(min.When), 0).Sub(time.Now()))
	}

	close(inited)
	for {
		select {
		case item := <-stage:
			if min == nil {
				// do nothing
			} else if item.When < min.When {
				heap.Push(&q, min)
			} else if item.When > min.When {
				heap.Push(&q, item)
				break
			}

			min = item
			PersistFunc(min)

			until := time.Unix(int64(min.When), 0)
			dur := until.Sub(time.Now())
			timer.Reset(dur)

		case <-timer.C:
			if min != nil {
				go func(item *Item) {
					RemoveFunc(item)
					RemindFunc(item)
				}(min)
			}

			if q.Len() == 0 {
				min = nil
				// fmt.Println("no item in heap")
				timer.Reset(24 * time.Hour)
				break
			}

			min = heap.Pop(&q).(*Item)

			until := time.Unix(int64(min.When), 0)
			dur := until.Sub(time.Now())
			timer.Reset(dur)
		}
	}
}
