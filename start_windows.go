package launcher

import (
	"flag"
	"path/filepath"
	"strings"

	"github.com/kardianos/service"

	"github.com/alrusov/config"
	"github.com/alrusov/log"
	"github.com/alrusov/misc"
	"github.com/alrusov/winservice"
)

var (
	serviceCommand = flag.String("service", "", "Service control: "+strings.Join(service.ControlAction[0:], ", "))
)

//----------------------------------------------------------------------------------------------------------------------------//

func start(a Application, cc *config.Common) {
	interactive := service.Interactive()
	tp := "a service"
	if interactive {
		tp = "an application"
	}

	log.Message(log.INFO, "Started as %s", tp)

	if interactive && *serviceCommand == "" {
		processor(a, cc)
		return
	}

	cfgFile, _ := filepath.Abs(*configFile)
	servConfig := &service.Config{
		Name:        cc.Name,
		DisplayName: cc.Name,
		Description: cc.Description,
		Arguments: []string{
			"--config", cfgFile,
		},
	}

	handler := func(serv *service.Service) {
		processor(a, cc)
	}

	serv, err := winservice.New(servConfig, handler)
	if err != nil {
		log.Message(log.CRIT, "Service initializtion error: %s", err.Error())
		misc.StopApp(10)
		return
	}

	err = serv.Go(*serviceCommand)
	if err != nil {
		log.Message(log.CRIT, "%s", err)
		code := 12
		if strings.Contains(err.Error(), "Access is denied") {
			log.Message(log.CRIT, "Try to run as administrator")
			code = 13
		}
		misc.StopApp(code)
		return
	}
}

//----------------------------------------------------------------------------------------------------------------------------//
