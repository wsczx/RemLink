package utils

import "sync"

type WorkerPool struct {
	JobQueue chan func()
	wg       sync.WaitGroup
}

func NewWorkerPool(workers, queue int) *WorkerPool {
	p := &WorkerPool{JobQueue: make(chan func(), queue)}
	p.wg.Add(workers)
	for range workers {
		go func() {
			defer p.wg.Done()
			for j := range p.JobQueue {
				j()
			}
		}()
	}
	return p
}
func (p *WorkerPool) Release() {
	close(p.JobQueue)
	p.wg.Wait()
}
