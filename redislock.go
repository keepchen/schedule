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

// SetRedisProviderClient 设置redis连接实例(单实例)
func SetRedisProviderClient(client *redisLib.Client) {
	doOnce.Do(func() {
		lockDriver.client = client
	})
}

// SetRedisProviderClusterClient 设置redis连接实例(cluster集群)
func SetRedisProviderClusterClient(client *redisLib.ClusterClient) {
	doOnce.Do(func() {
		lockDriver.clusterClient = client
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

func (rd *redisDriver) tryLock(key string) bool {
	if rd.client == nil && rd.clusterClient == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), redisExecuteTimeout)
	defer cancel()

	var (
		ok  bool
		err error
	)

	if rd.client != nil {
		ok, err = rd.client.SetNX(ctx, key, lockerValue(), lockTTL).Result()
	}

	if rd.clusterClient != nil {
		ok, err = rd.clusterClient.SetNX(ctx, key, lockerValue(), lockTTL).Result()
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
					if rd.client != nil {
						if redisOK, redisErr := rd.client.Expire(innerCtx, key, lockTTL).Result(); !redisOK || redisErr != nil {
							break LOOP
						}
					}
					if rd.clusterClient != nil {
						if redisOK, redisErr := rd.clusterClient.Expire(innerCtx, key, lockTTL).Result(); !redisOK || redisErr != nil {
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

func (rd *redisDriver) unlock(key string) {
	if rd.client == nil && rd.clusterClient == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), redisExecuteTimeout)
	defer cancel()

	if rd.client != nil {
		_, _ = rd.client.Del(ctx, key).Result()
	}

	if rd.clusterClient != nil {
		_, _ = rd.clusterClient.Del(ctx, key).Result()
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
