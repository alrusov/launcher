package launcher

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/alrusov/config"
	"github.com/alrusov/jsonw"
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
	flagConfigFile   = flag.String("config", "", "Configuration file to use")
	flagVersion      = flag.Bool("version", false, "Daemon version")
	flagDumpPanicIDs = flag.Bool("dump-panic-ids", false, "Dump panic IDs to log with TRACE3 level")
)

//----------------------------------------------------------------------------------------------------------------------------//

// Go --
func Go(a Application, cfg interface{}) {
	flag.Parse()

	if *flagDumpPanicIDs {
		panic.SetDumpStack(true)
	}

	panicID := panic.ID()
	defer panic.SaveStackToLogEx(panicID)

	misc.Logger = log.StdLogger

	if *flagVersion {
		ts := misc.BuildTime()
		if ts != "" {
			ts = " [" + ts + "Z]"
		}

		tags := misc.AppTags()
		if tags != "" {
			tags = " " + tags
		}

		fmt.Fprintf(os.Stderr, "%s %s%s%s, %s/%s\n%s\n", misc.AppName(), misc.AppVersion(), tags, ts, runtime.GOOS, runtime.GOARCH, misc.Copyright())
		os.Exit(misc.ExVersion)
	}

	if *flagConfigFile == "" {
		log.Message(log.ALERT, "Missing configuration file\nUse:\n")
		flag.PrintDefaults()
		os.Exit(misc.ExMissingConfigFile)
	}

	if err := config.LoadFile(*flagConfigFile, cfg); err != nil {
		log.Message(log.ALERT, "Incorrect config file: %s", err)
		misc.StopApp(misc.ExIncorrectConfigFile)
		misc.Exit()
	}

	cc := config.GetCommon()
	if cc == nil {
		log.Message(log.ALERT, "Config has an incorrect structure\n")
		misc.StopApp(misc.ExConfigIncorrect)
		misc.Exit()
	}

	jsonw.UseStd(cc.UseStdJSON)

	log.MaxLen(cc.LogMaxStringLen)
	log.SetFile(cc.LogDir, "", cc.LogLocalTime, cc.LogBufferSize, cc.LogBufferDelay)
	log.SetLogLevels(cc.LogLevel, cc.LogLevels, log.FuncNameModeNone)
	log.Message(log.DEBUG, "Config file:\n>>>\n%s\n<<<", string(config.GetSecuredText()))

	if err := a.CheckConfig(); err != nil {
		log.Message(log.ALERT, "Config errors: %s", err.Error())
		misc.StopApp(misc.ExConfigErrors)
		misc.Exit()
	}

	if cc.GoMaxProcs > 0 {
		runtime.GOMAXPROCS(cc.GoMaxProcs)
	}

	if cc.DeepProfiling {
		runtime.SetBlockProfileRate(1)
		runtime.SetMutexProfileFraction(1)
	}

	start(a, cc)
	misc.Exit()
}

//----------------------------------------------------------------------------------------------------------------------------//

func processor(a Application, cc *config.Common) {
	stdhttp.SetMinSizeForGzip(cc.MinSizeForGzip)

	listener, err := a.NewListener()
	if err != nil {
		log.Message(log.CRIT, "Create listener error: %s", err.Error())
		misc.StopApp(misc.ExCreateListenerError)
		return
	}

	listener.SetName(cc.Name, cc.Description)
	go memStats(cc)

	go func() {
		panicID := panic.ID()
		defer panic.SaveStackToLogEx(panicID)
		misc.WaitingForStop()
		log.Message(log.INFO, "Stopping...")
		listener.Stop()
	}()

	log.Message(log.INFO, "Press Ctrl+C for exit")

	if err := listener.Start(); err != nil {
		log.Message(log.CRIT, "Start listener error: %s", err.Error())
		misc.StopApp(misc.ExStartListenerError)
		return
	}

	return
}

//----------------------------------------------------------------------------------------------------------------------------//

func memStats(cc *config.Common) {
	panicID := panic.ID()
	defer panic.SaveStackToLogEx(panicID)

	if cc.MemStatsPeriod > 0 {
		level, ok := log.Str2Level(cc.MemStatsLevel)
		if !ok {
			level = log.DEBUG
		}

		if level <= log.CurrentLogLevel() {
			delay := time.Duration(cc.MemStatsPeriod) * time.Second
			var mem runtime.MemStats

			for {
				//debug.FreeOSMemory()
				misc.Sleep(1 * time.Second)
				runtime.ReadMemStats(&mem)
				log.Message(level, "AllocSys %d, HeapSys %d, HeapInuse: %d, HeapObjects %d, StackSys: %d, StackInuse: %d; NumCPU: %d; GoMaxProcs: %d; NumGoroutine: %d",
					mem.Sys, mem.HeapSys, mem.HeapInuse, mem.HeapObjects, mem.StackSys, mem.StackInuse, runtime.NumCPU(), runtime.GOMAXPROCS(-1), runtime.NumGoroutine())
				if !misc.Sleep(delay) {
					break
				}
			}
		}
	}
}

//----------------------------------------------------------------------------------------------------------------------------//
