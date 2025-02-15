package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/pkg"
	d "github.com/Karmenzind/kd/pkg/decorate"
	"github.com/Karmenzind/kd/pkg/proc"

	"github.com/shirou/gopsutil/v3/process"
	"go.uber.org/zap"
)

type DaemonInfoType struct {
	StartTime int64
	Port      string
	PID       int
}

var DaemonInfo = &DaemonInfoType{}

func RecordRunInfo(port string) {
	DaemonInfo.StartTime = time.Now().Unix()
	DaemonInfo.PID = os.Getpid()
	DaemonInfo.Port = port
	pkg.SaveJson(
		filepath.Join(cache.CACHE_RUN_PATH, "daemon.json"),
		DaemonInfo,
	)
	zap.S().Infof("Recorded running information of daemon %+v", DaemonInfo)
}

func GetDaemonInfo() *DaemonInfoType {
	if *DaemonInfo == (DaemonInfoType{}) {
		err := pkg.LoadJson(filepath.Join(cache.CACHE_RUN_PATH, "daemon.json"), DaemonInfo)
		if err != nil {
			d.EchoFatal("获取守护进程信息失败，请执行`kd --stop && kd --daemon`")
		}
	}
	return DaemonInfo
}

func getKdPIDs() {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("taskkill", "/NH", "/FO", "csv")
	}
	output, err := cmd.Output()
	if err != nil {
		_ = err
	}
	strings.Split(string(output), "\n")
}

func ServerIsRunning() bool {
	p, _ := FindServerProcess()
	return p != nil
}

func FindServerProcess() (*process.Process, error) {
	processes, err := process.Processes()
	if err != nil {
		return nil, err
	}
	di := GetDaemonInfo()
	for _, p := range processes {
		// XXX err
		n, _ := p.Name()
		if p.Pid == int32(di.PID) {
            zap.S().Debugf("Got daemon process %v via daemon info", di.PID)
			cmdslice, _ := p.CmdlineSlice()
			if len(cmdslice) > 1 && cmdslice[1] == "--server" {
				return p, nil
			}
		}

		if n == "kd" || (runtime.GOOS == "windows" && n == "kd.exe") {
			cmd, _ := p.Cmdline()
			if p.Pid == 13328 {
				name, _ := p.Name()
				cmdslice, _ := p.CmdlineSlice()
				zap.S().Debugf("13328:Name: `%s` Cmd: `%s` cmdslice: `%+v`", name, cmd, cmdslice)
			}
			zap.S().Debugf("Found process kd.exe with CMD: %s", cmd)
			if strings.Contains(cmd, " --server") {
				zap.S().Debugf("Found process %+v Cmd: `%s`", p, cmd)
				return p, nil
			}
		}
	}
	return nil, nil
}

func StartDaemonProcess() error {
	kdpath, err := pkg.GetExecutablePath()
	if err != nil {
		zap.S().Errorf("Failed to get current file path: %s", err)
		return err
	}
	zap.S().Debugf("Got executable path %s", kdpath)

	cmd := exec.Command(kdpath, "--server")
	err = cmd.Start()
	if err != nil {
		zap.S().Errorf("Failed to start daemon with system command: %s", err)
		return err
	}
	var p *process.Process
	for i := 0; i < 3; i++ {
		time.Sleep(time.Second)
		p, err_ := FindServerProcess()
		if err_ != nil {
			zap.S().Warnf("Failed finding daemon process: %s", err_)
		}
		if p != nil {
			zap.S().Infof("Started daemon process.")
			d.EchoOkay(fmt.Sprintf("成功启动守护进程，PID：%d", p.Pid))
			return nil
		}
		d.EchoRun("正在检查运行结果，稍等...")
	}
	if p == nil {
		err = fmt.Errorf("启动失败，请重试。如果多次启动失败，请创建Issue并提交日志文件")
		return err
	}
	return nil
}

func KillDaemonIfRunning() error {
	p, err := FindServerProcess()
	if err == nil {
		if p == nil {
			d.EchoOkay("未发现守护进程，无需停止")
			return nil
		}
	} else {
		zap.S().Warnf("[process] Failed to find daemon: %s", err)
		return err
	}

    err = proc.KillProcess(p)

	if err == nil {
		zap.S().Info("Terminated daemon process.")
		d.EchoOkay("守护进程已经停止")
	}
	return err
}
