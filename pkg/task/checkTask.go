package task

import (
	"golang.org/x/net/context"
	deafault "pic_offload/pkg/apis"
	"time"
)

func (ts *TaskScheduler) CheckTasks() {

	ctx, _ := context.WithCancel(context.Background())
	go func() {
		t := time.NewTicker(deafault.CheckTaskInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				for _, i := range ts.Tasks {
					if i.Done == false {
						//TODO do this task
						
					}
				}
			}
		}
	}()
	return
}
