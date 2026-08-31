package base

import "sync"

func Start() {
	execute()
	initCfg()
	initLog()
	initContainer()
}

var once sync.Once

func Test() {
	once.Do(func() {
		initLog()
	})
}
