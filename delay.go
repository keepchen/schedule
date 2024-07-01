package schedule

import (
	"time"
)

// RunOnceTimeAfter 在一定时间后执行
//
// # Note
//
// 这是一个一次性任务，只会执行一次，不会重复执行
func (j *taskJob) RunOnceTimeAfter(delay time.Duration) (cancel CancelFunc) {
	timer := time.After(delay)
	cancel = j.cancelFunc

	j.wrappedTaskFunc = func() {
		j.running = true

		defer func() {
			j.running = false
		}()

		if !j.withoutOverlapping {
			j.task()
			return
		}
		if lockDriver.tryLock(j.lockerKey) {
			defer func() {
				lockDriver.unlock(j.lockerKey)
				j.lockedByMe = false
			}()
			j.lockedByMe = true
			j.task()
		}
	}

	go func() {
	LOOP:
		for {
			select {
			case <-timer:
				go j.wrappedTaskFunc()
				break LOOP
			case <-j.cancelTaskChan:
				break LOOP
			}
		}
	}()

	return cancel
}
