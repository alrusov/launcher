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

	_ "time/tzdata" // If the time package cannot find tzdata files on the system, it will use this embedded information

	authbasic "github.com/alrusov/auth-basic"
	authjwt "github.com/alrusov/auth-jwt"
	authkeycloak "github.com/alrusov/auth-keycloak"
	authkrb5 "github.com/alrusov/auth-krb5"
	authurl "github.com/alrusov/auth-url"
)

//----------------------------------------------------------------------------------------------------------------------------//

// Application --
type Application interface {
	CheckConfig() error
	NewListener() (*stdhttp.HTTP, error)
}

var (
	cfgFile string
)

//----------------------------------------------------------------------------------------------------------------------------//

// Go --
func Go(a Application, cfg interface{}) {
	defaultConfig, _ := misc.AbsPath(fmt.Sprintf("%s/%s.toml", misc.AppExecPath(), misc.AppName()))

	flagEnvFile := flag.String("env", misc.DefaultEnvFile, "Environment file to use")
	flagConfigFile := flag.String("config", defaultConfig, "Configuration file to use")
	flagVersion := flag.Bool("version", false, "Daemon version")
	flagDumpPanicIDs := flag.Bool("dump-panic-ids", false, "Dump panic IDs to log with ALERT level")
	flagDisableConsoleLog := flag.Bool("disable-console-log", false, "Disable console log")
	flagDebug := flag.Bool("debug", false, "Debug mode")

	flag.Parse()

	if *flagDebug {
		panic.Disable()
	}

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
		return // formally for validators
	}

	var err error
	cfgFile, err = misc.AbsPath(*flagConfigFile)
	if err != nil {
		log.Message(log.ALERT, "Incorrect config file name: %s", err)
		misc.StopApp(misc.ExIncorrectConfigFile)
		misc.Exit()
		return // formally for validators
	}

	if err := misc.LoadEnv(*flagEnvFile); err != nil {
		log.Message(log.ALERT, "Incorrect environment file: %s", err)
		misc.StopApp(misc.ExIncorrectConfigFile)
		misc.Exit()
		return // formally for validators
	}

	if err := config.LoadFile(cfgFile, cfg); err != nil {
		log.Message(log.ALERT, "Incorrect config file: %s", err)
		misc.StopApp(misc.ExIncorrectConfigFile)
		misc.Exit()
		return // formally for validators
	}

	cc := config.GetCommon()
	if cc == nil {
		log.Message(log.ALERT, "Config has an incorrect structure\n")
		misc.StopApp(misc.ExConfigIncorrect)
		misc.Exit()
		return // formally for validators
	}

	jsonw.UseStd(cc.UseStdJSON)

	if *flagDisableConsoleLog {
		log.SetConsoleWriter(nil)
	}

	log.MaxLen(cc.LogMaxStringLen)
	log.SetFile(cc.LogDir, "", cc.LogLocalTime, cc.LogBufferSize, time.Duration(cc.LogBufferDelay))
	log.SetLogLevels(cc.LogLevel, cc.LogLevels, log.FuncNameModeNone)
	log.Message(log.DEBUG, "Config file:\n>>>\n%s\n<<<", string(config.GetSecuredText()))

	if err := a.CheckConfig(); err != nil {
		log.Message(log.ALERT, "Config errors: %s", err.Error())
		misc.StopApp(misc.ExConfigErrors)
		misc.Exit()
		return // formally for validators
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

	err = addAuth(listener)
	if err != nil {
		log.Message(log.CRIT, "Auth initialization: %s", err.Error())
		misc.StopApp(misc.ExCreateListenerError)
		return
	}

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
}

//----------------------------------------------------------------------------------------------------------------------------//

func addAuth(listener *stdhttp.HTTP) (err error) {
	err = authbasic.Add(listener)
	if err != nil {
		return err
	}

	err = authjwt.Add(listener)
	if err != nil {
		return err
	}

	err = authkrb5.Add(listener)
	if err != nil {
		return err
	}

	err = authkeycloak.Add(listener)
	if err != nil {
		return err
	}

	err = authurl.Add(listener)
	if err != nil {
		return err
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
			delay := cc.MemStatsPeriod
			var mem runtime.MemStats

			for {
				//debug.FreeOSMemory()
				misc.Sleep(1 * time.Second)
				runtime.ReadMemStats(&mem)
				log.Message(level, "AllocSys %d, HeapSys %d, HeapInuse: %d, HeapObjects %d, StackSys: %d, StackInuse: %d; NumCPU: %d; GoMaxProcs: %d; NumGoroutine: %d",
					mem.Sys, mem.HeapSys, mem.HeapInuse, mem.HeapObjects, mem.StackSys, mem.StackInuse, runtime.NumCPU(), runtime.GOMAXPROCS(-1), runtime.NumGoroutine())
				if !misc.Sleep(time.Duration(delay)) {
					break
				}
			}
		}
	}
}

//----------------------------------------------------------------------------------------------------------------------------//
