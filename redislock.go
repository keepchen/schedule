package schedule

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	redisLib "github.com/go-redis/redis/v8"
)

type redisDriver struct {
	client        *redisLib.Client
	clusterClient *redisLib.ClusterClient
}

var (
	lockDriver  = &redisDriver{}
	pingTimeout = time.Second * 3
	doOnce      = &sync.Once{}
)

// SetRedisProviderStandalone 设置redis连接配置(standalone)
func SetRedisProviderStandalone(opt *redisLib.Options) {
	doOnce.Do(func() {
		initClient(opt)
	})
}

// SetRedisProviderCluster 设置redis连接配置(cluster)
func SetRedisProviderCluster(opt *redisLib.ClusterOptions) {
	doOnce.Do(func() {
		initClusterClient(opt)
	})
}

// SetRedisProviderFailOver 设置redis连接配置(fail-over)
func SetRedisProviderFailOver(opt *redisLib.FailoverOptions) {
	doOnce.Do(func() {
		initFailOverClient(opt)
	})
}

// ReleaseRedisProvider 释放redis连接
func ReleaseRedisProvider() {
	if lockDriver.client != nil {
		_ = lockDriver.client.Close()
	}
	if lockDriver.clusterClient != nil {
		_ = lockDriver.clusterClient.Close()
	}
}

func initClient(opt *redisLib.Options) {
	rdb := redisLib.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	err := rdb.Ping(ctx).Err()
	if err != nil {
		panic(err)
	}
	cancel()

	lockDriver.client = rdb
}

func initClusterClient(opt *redisLib.ClusterOptions) {
	rdb := redisLib.NewClusterClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	err := rdb.Ping(ctx).Err()
	if err != nil {
		panic(err)
	}
	cancel()

	lockDriver.clusterClient = rdb
}

func initFailOverClient(opt *redisLib.FailoverOptions) {
	rdb := redisLib.NewFailoverClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	err := rdb.Ping(ctx).Err()
	if err != nil {
		panic(err)
	}
	cancel()

	lockDriver.client = rdb
}

type stateListeners struct {
	mux       *sync.Mutex
	listeners map[string]chan struct{}
}

var (
	lockTTL              = time.Second * 10
	redisExecuteTimeout  = time.Second * 3
	renewalCheckInterval = time.Second * 1
	states               = &stateListeners{mux: &sync.Mutex{}, listeners: make(map[string]chan struct{})}
)

func (r *redisDriver) tryLock(key string) bool {
	if r.client == nil && r.clusterClient == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), redisExecuteTimeout)
	go func() {
		for range time.After(redisExecuteTimeout) {
			cancel()
			break
		}
	}()

	var (
		ok  bool
		err error
	)

	if r.client != nil {
		ok, err = r.client.SetNX(ctx, key, lockerValue(), lockTTL).Result()
	}

	if r.clusterClient != nil {
		ok, err = r.clusterClient.SetNX(ctx, key, lockerValue(), lockTTL).Result()
	}

	if err != nil {
		return false
	}

	if ok {
		cancelChan := make(chan struct{})

		//自动续期
		go func() {
			ticker := time.NewTicker(renewalCheckInterval)
			innerCtx := context.Background()
			defer ticker.Stop()

		LOOP:
			for {
				select {
				case <-ticker.C:
					if r.client != nil {
						if redisOK, redisErr := r.client.Expire(innerCtx, key, lockTTL).Result(); !redisOK || redisErr != nil {
							break LOOP
						}
					}
					if r.clusterClient != nil {
						if redisOK, redisErr := r.clusterClient.Expire(innerCtx, key, lockTTL).Result(); !redisOK || redisErr != nil {
							break LOOP
						}
					}
				case <-cancelChan:
					break LOOP
				}
			}
		}()

		states.mux.Lock()
		states.listeners[key] = cancelChan
		states.mux.Unlock()
	}

	return ok
}

func (r *redisDriver) unlock(key string) {
	if r.client == nil && r.clusterClient == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), redisExecuteTimeout)

	go func() {
		for range time.After(redisExecuteTimeout) {
			cancel()
			break
		}
	}()

	if r.client != nil {
		_, _ = r.client.Del(ctx, key).Result()
	}

	if r.clusterClient != nil {
		_, _ = r.clusterClient.Del(ctx, key).Result()
	}

	go func() {
		states.mux.Lock()
		ch, ok := states.listeners[key]
		if ok {
			delete(states.listeners, key)
		}
		states.mux.Unlock()
		if ok {
			ch <- struct{}{}
			close(ch)
		}
	}()
}

// 锁的持有者信息
func lockerValue() string {
	hostname, _ := os.Hostname()
	ip, _ := GetLocalIP()

	return fmt.Sprintf("lockedAt:%s@%s(%s)", time.Now().Format("2006-01-02T15:04:05Z"), hostname, ip)
}
