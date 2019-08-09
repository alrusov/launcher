package launcher

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"

	"github.com/alrusov/config"
	"github.com/alrusov/log"
	"github.com/alrusov/misc"
	"github.com/alrusov/panic"
	"github.com/alrusov/stdhttp"
)

//----------------------------------------------------------------------------------------------------------------------------//

// Application --
type Application interface {
	CheckConfig() error
	CommonConfig() *config.Common
	NewListener() (*stdhttp.HTTP, error)
}

var (
	configFile = flag.String("config", "", "Configuration file to use")
	flversion  = flag.Bool("version", false, "Daemon version")
)

//----------------------------------------------------------------------------------------------------------------------------//

func memStats(cc *config.Common) {
	defer panic.SaveStackToLog()

	if cc.MemStatsPeriod > 0 {
		level, ok := log.Str2Level(cc.MemStatsLevel)
		if !ok {
			level = log.DEBUG
		}

		currlogLevel, _, _ := log.GetCurrentLogLevel()
		if level <= currlogLevel {
			delay := time.Duration(cc.MemStatsPeriod) * time.Second
			var mem runtime.MemStats

			for {
				runtime.ReadMemStats(&mem)
				log.Message(level, "AllocSys %d, HeapSys %d, HeapAlloc %d; NumCPU: %d; GoMaxProcs: %d; NumGoroutine: %d",
					mem.Sys, mem.HeapSys, mem.HeapAlloc, runtime.NumCPU(), runtime.GOMAXPROCS(-1), runtime.NumGoroutine())
				if !misc.Sleep(delay) {
					break
				}
			}
		}
	}
}

//----------------------------------------------------------------------------------------------------------------------------//

// Go --
func Go(a Application, cfg interface{}) {
	defer panic.SaveStackToLog()

	misc.Logger = log.StdLogger

	flag.Parse()

	if *flversion {
		ts := misc.BuildTime()
		if ts != "" {
			ts = " [" + ts + "]"
		}
		fmt.Fprintf(os.Stdout, "%s %s%s\n%s\n", misc.AppName(), misc.AppVersion(), ts, misc.Copyright())
		syscall.Exit(1)
	} else if *configFile == "" {
		fmt.Fprintf(os.Stdout, "Missing configuration file\nUse:\n")
		flag.PrintDefaults()
		syscall.Exit(2)
	} else if err := config.LoadFile(*configFile, cfg); err != nil {
		log.Message(log.ALERT, "Incorrect config file: %s", err)
		misc.StopApp(3)
	} else {
		cc := a.CommonConfig()
		log.SetFile(cc.LogDir, "", cc.LogLocalTime, cc.LogBufferSize, cc.LogBufferDelay)
		log.SetCurrentLogLevel(cc.LogLevel, "")

		if err := a.CheckConfig(); err != nil {
			log.Message(log.ALERT, "Config errors: %s", err.Error())
			misc.StopApp(4)
		} else {
			if cc.GoMaxProcs > 0 {
				runtime.GOMAXPROCS(cc.GoMaxProcs)
			}

			if listener, err := a.NewListener(); err != nil {
				log.Message(log.CRIT, "Create listener error: %s", err.Error())
			} else {
				go memStats(cc)

				go func() {
					defer panic.SaveStackToLog()
					misc.WaitingForStop()
					log.Message(log.INFO, "Stopping...")
					listener.Stop()
				}()

				if err := listener.Start(); err != nil {
					log.Message(log.CRIT, "Start listener error: %s", err.Error())
				}
			}
		}
	}

	misc.Exit()
}

//----------------------------------------------------------------------------------------------------------------------------//
