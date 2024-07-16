# Schedule  
[![Go](https://github.com/keepchen/schedule/actions/workflows/go.yml/badge.svg)](https://github.com/keepchen/schedule/actions/workflows/go.yml)  [![CodeQL](https://github.com/keepchen/schedule/actions/workflows/codeql.yml/badge.svg)](https://github.com/keepchen/schedule/actions/workflows/codeql.yml)  [![Go Report Card](https://goreportcard.com/badge/github.com/keepchen/schedule)](https://goreportcard.com/report/github.com/keepchen/schedule)  
A distributed scheduled task tool written in Go. It was incubated and evolved from the [go-sail](https://github.com/keepchen/go-sail) framework.  

### Requirement  
```text
go version >= 1.19
```

### Installation  
```shell
go get -u github.com/keepchen/schedule
```  

### Features  
- [x] Interval task  
- [x] Once time task  
- [x] Linux Crontab Style task  
- [x] Cancelable  
- [x] Race detection  
- [x] Manual call

### Examples  
#### Interval  
```go
schedule.NewJob("say hello").EveryMinute()
```  
#### Once time  
```go
schedule.NewJob("check in").RunOnceTimeAfter(time.Second)
```  
#### Linux Crontab Style  
```go
schedule.NewJob("good morning").RunAt(schedule.EveryDayOfEightAMClock)
```  
#### Cancelable
```go
cancel := schedule.NewJob("say hello").EveryMinute()

time.AfterFunc(time.Minute*3, cancel)
```  
#### Race detection  
> Note: You must set redis provider before use.
```go  
// set redis provider
schedule.SetRedisProviderStandalone(...)
// or
schedule.SetRedisProviderClient(...)

schedule.NewJob("say hello").WithoutOverlapping().EveryMinute()
```  
#### Manual call
```go
schedule.Call("say hello", false)

schedule.MustCall("task not exist will be panic", true)
```  
