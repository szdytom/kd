package internal

/*

功能：

- 查询
- 更新
*/

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/core"
	"github.com/Karmenzind/kd/internal/daemon"
	"github.com/Karmenzind/kd/internal/model"
	q "github.com/Karmenzind/kd/internal/query"
	"github.com/Karmenzind/kd/pkg"
	d "github.com/Karmenzind/kd/pkg/decorate"
	"github.com/Karmenzind/kd/pkg/proc"
	"github.com/Karmenzind/kd/pkg/str"
	"go.uber.org/zap"
)

func ensureDaemon(running chan bool) {
	p, _ := daemon.FindServerProcess()
	var err error
	if p == nil {
		d.EchoRun("未找到守护进程，正在启动...")
		err = daemon.StartDaemonProcess()
		if err != nil {
			d.EchoFatal(err.Error())
		}
	} else {
		exename, err := pkg.GetExecutablePath()
		if err == nil {
			runningExename, _ := p.Exe()
			// TODO (k): <2024-01-03> 增加检查版本
			if exename != runningExename {
				d.EchoWarn(fmt.Sprintf("正在运行的守护程序（%s）与当前程序（%s）文件信息或版本不一致，将尝试重新启动守护进程", runningExename, exename))
				err := proc.KillProcess(p)
				if err != nil {
					cmd := proc.GetKillCMD(p.Pid)
					d.EchoError(fmt.Sprintf("停止进程%v失败，请手动执行：", p.Pid))
					fmt.Println(cmd.String())
					os.Exit(1)
				}
				d.EchoRun("已终止，正在启动新的守护进程...")
				err = daemon.StartDaemonProcess()
				if err != nil {
					d.EchoFatal(err.Error())
				}
			}
		}
	}
	// if !daemon.ServerIsRunning() {
	// 	err := daemon.StartDaemonProcess()
	// 	if err != nil {
	// 		d.EchoRun("未找到守护进程，正在启动...")
	// 		d.EchoFatal(err.Error())
	// 	}
	// 	running <- true
	// }
	running <- true
}

func Query(query string, noCache bool, longText bool) (r *model.Result, err error) {
	// TODO (k): <2024-01-02> regexp
	query = str.Simplify(query)
	if !longText {
		query = strings.ToLower(query)
	}
	// query = strings.ToLower(strings.Trim(query, " "))
	// query = strings.ReplaceAll(query, "\n", " ")

	r = buildResult(query, longText)
	r.History = make(chan int, 1)

	daemonRunning := make(chan bool)
	go ensureDaemon(daemonRunning)

	core.WG.Add(1)
	go cache.CounterIncr(query, r.History)

	// any valid char
	if m, _ := regexp.MatchString("^[a-zA-Z0-9\u4e00-\u9fa5]", query); !m {
		r.Found = false
		r.Prompt = "请输入有效查询字符或参数"
		return
	}

	// if longText {
	// 	r.Found = false
	// 	r.Prompt = "暂不支持长句翻译"
	// 	return
	// }

	var inNotFound bool
	var line int
	if !longText {
		line, err = cache.CheckNotFound(r.Query)
		if err != nil {
			zap.S().Warnf("[cache] check not found error: %s", err)
		} else if line > 0 {
			if !noCache {
				r.Found = false
				zap.S().Debugf("`%s` is in not-found-list", r.Query)
				return
			}
			inNotFound = true
		}
		r.Initialize()

		if !noCache {
			cacheErr := q.FetchCached(r)
			if cacheErr != nil {
				zap.S().Warnf("[cache] Query error: %s", cacheErr)
			}
			if r.Found {
				return
			}
			_ = err
		}

	}

	if <-daemonRunning {
		err = QueryDaemon(r)
		if err == nil && r.Found && inNotFound {
			go cache.RemoveNotFound(r.Query)
		}
	} else {
		d.EchoFatal("守护进程未启动，请手动执行`kd --daemon`")
	}

	// FIXME move to server
	// if !r.Found {
	// 	err = q.FetchOnline(r)
	// 	// 判断时间
	// 	cache.UpdateQueryCache(r)
	// }
	return r, err
}

func QueryDaemon(r *model.Result) error {
	addr := fmt.Sprintf("localhost:%d", SERVER_PORT)
	err := q.QueryDaemon(addr, r)
	return err
}
