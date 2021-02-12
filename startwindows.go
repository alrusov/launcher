// +build windows

package launcher

import (
	"flag"
	"strings"

	"github.com/kardianos/service"

	"github.com/alrusov/config"
	"github.com/alrusov/log"
	"github.com/alrusov/misc"
	"github.com/alrusov/winservice"
)

var (
	serviceCommand = flag.String("service", "", "Service control (administrative rights required): "+strings.Join(service.ControlAction[0:], ", "))
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

	cfgFile, _ := misc.AbsPath(*flagConfigFile)
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
		log.Message(log.CRIT, "Service initialization error: %s", err.Error())
		misc.StopApp(misc.ExServiceInitializationError)
		return
	}

	err = serv.Go(*serviceCommand)
	if err != nil {
		log.Message(log.CRIT, "%s", err)
		code := misc.ExServiceError
		if strings.Contains(err.Error(), "Access is denied") {
			log.Message(log.CRIT, "Try to run as the Administrator")
			code = misc.ExAccessDenied
		}
		misc.StopApp(code)
		return
	}
}

//----------------------------------------------------------------------------------------------------------------------------//
