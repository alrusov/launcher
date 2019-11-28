package launcher

import (
	"flag"
	"fmt"
	"os"
	"runtime"
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
				log.Message(level, "AllocSys %d, HeapSys %d, StackSys: %d; NumCPU: %d; GoMaxProcs: %d; NumGoroutine: %d",
					mem.Sys, mem.HeapSys, mem.StackSys, runtime.NumCPU(), runtime.GOMAXPROCS(-1), runtime.NumGoroutine())
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
		fmt.Fprintf(os.Stderr, "%s %s%s\n%s\n", misc.AppName(), misc.AppVersion(), ts, misc.Copyright())
		os.Exit(1)
	} else if *configFile == "" {
		fmt.Fprintf(os.Stderr, "Missing configuration file\nUse:\n")
		flag.PrintDefaults()
		os.Exit(2)
	} else if err := config.LoadFile(*configFile, cfg); err != nil {
		log.Message(log.ALERT, "Incorrect config file: %s", err)
		os.Exit(3)
	} else {
		cc := config.GetCommon()
		if cc == nil {
			fmt.Fprintf(os.Stderr, "Config not loaded\n")
			os.Exit(4)
		}

		log.SetFile(cc.LogDir, "", cc.LogLocalTime, cc.LogBufferSize, cc.LogBufferDelay)
		log.SetCurrentLogLevel(cc.LogLevel, "")
		log.Message(log.DEBUG, "Config file:\n%s", string(config.GetText()))

		if err := a.CheckConfig(); err != nil {
			log.Message(log.ALERT, "Config errors: %s", err.Error())
			misc.StopApp(5)
		} else {
			if cc.GoMaxProcs > 0 {
				runtime.GOMAXPROCS(cc.GoMaxProcs)
			}

			if cc.DeepProfiling {
				runtime.SetBlockProfileRate(1)
				runtime.SetMutexProfileFraction(1)
			}

			if listener, err := a.NewListener(); err != nil {
				log.Message(log.CRIT, "Create listener error: %s", err.Error())
			} else {
				stdhttp.SetName(cc.Name, cc.Description)
				go memStats(cc)

				go func() {
					defer panic.SaveStackToLog()
					misc.WaitingForStop()
					log.Message(log.INFO, "Stopping...")
					listener.Stop()
				}()

				log.Message(log.INFO, "Press Ctrl+C for exit")

				if err := listener.Start(); err != nil {
					log.Message(log.CRIT, "Start listener error: %s", err.Error())
				}
			}
		}
	}

	misc.Exit()
}

//----------------------------------------------------------------------------------------------------------------------------//
