// +build windows

package initme

import (
    "fmt"
    "time"
    "os/exec"
    "syscall"
    "errors"

    "golang.org/x/sys/windows/svc"
)

func init() {
    serviceType = WindowsService{}
}

type WindowsService struct {
    Conf Config
}

func (self WindowsService) New(c Config) Service {

    self.Conf = c

    return self
}

func (self WindowsService) Register() (output string, err error, code int)  {
    args, err := self.buildScArgs("create")
    if err != nil {
        return
    }
    return self.execute(args...)
}

func (self WindowsService) Start() (output string, err error, code int)  {
    return self.execute("start", self.Conf.Name)
}

func (self WindowsService) Stop() (output string, err error, code int)  {
    return self.execute("stop", self.Conf.Name)
}

func (self WindowsService) Status() (output string, err error, code int)  {
    return self.execute("query", self.Conf.Name)
}

func (self WindowsService) Disable() (output string, err error, code int)  {
    return self.execute("config", self.Conf.Name, "start=", "disabled")
}

func (self WindowsService) Delete() (output string, err error, code int)  {
    return self.execute("delete", self.Conf.Name)
}

// https://support.microsoft.com/en-us/kb/251192
func (self WindowsService) buildScArgs(init... string) (args []string, err error) {
    args = make([]string, 0)

    args = append(args, init...)

    if self.Conf.Name != "" {
        args = append(args, self.Conf.Name)
    } else {
        return nil, errors.New("Name is mandatory")
    }
    if self.Conf.Type != "" {
        args = append(args, "type=", self.Conf.Type)
    }
    if self.Conf.StartType != "" {
        args = append(args, "start=", self.Conf.StartType)
    }
    if self.Conf.Error != "" {
        args = append(args, "error=", self.Conf.Error)
    }
    if self.Conf.BinPath != "" {
        args = append(args, "binpath=", self.Conf.BinPath)
    } else {
        return nil, errors.New("BinPath is mandatory")
    }
    if self.Conf.Group != "" {
        args = append(args, "group=", self.Conf.Group)
    }
    if self.Conf.Tag != "" {
        args = append(args, "tag=", self.Conf.Tag)
    }
    if self.Conf.Depend != "" {
        args = append(args, "depend=", self.Conf.Depend)
    }
    if self.Conf.Obj != "" {
        if self.Conf.Password != "" {
            return nil, errors.New("Password is mandatory if Obj is set")
        }
        args = append(args, "obj=", self.Conf.Obj)
    }
    if self.Conf.DisplayName != "" {
        args = append(args, "DisplayName=", self.Conf.DisplayName)
    }
    if self.Conf.Password != "" {
        if self.Conf.Obj != "" {
            return nil, errors.New("Password is meanful only if Obj is set")
        }
        args = append(args, "password=", self.Conf.Password)
    }

    return
}

func (self WindowsService) execute(args... string) (output string, err error, code int) {
	self.Conf.Log.Print("sc.exe ")
	self.Conf.Log.Println(args)

    cmd := exec.Command("sc.exe", args...)
    var waitStatus syscall.WaitStatus
    var outputBytes []byte
    if outputBytes, err = cmd.CombinedOutput(); err != nil {
        // Did the command fail because of an unsuccessful exit code
        if exitError, ok := err.(*exec.ExitError); ok {
            waitStatus = exitError.Sys().(syscall.WaitStatus)
            code = waitStatus.ExitStatus()
        }
    } else {
        // Command was successful
        waitStatus = cmd.ProcessState.Sys().(syscall.WaitStatus)
        code = waitStatus.ExitStatus()
    }

	output = string(outputBytes)
	self.Conf.Log.Println(output, err, code)
    return
}

func (self WindowsService) IsAnInteractiveSession() (bool, error) {
    return svc.IsAnInteractiveSession()
}

func (self WindowsService) Run() {
    svc.Run(self.Conf.Name, self)
}

func (self WindowsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue
	changes <- svc.Status{State: svc.StartPending}
	fasttick := time.Tick(500 * time.Millisecond)
	slowtick := time.Tick(2 * time.Second)
	tick := fasttick
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

    go self.Conf.Job()

loop:
	for {
		select {
		case <-tick:

			//self.eventLog.Info(1, "beep")
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
				// Testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
				time.Sleep(100 * time.Millisecond)
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				break loop
			case svc.Pause:
				changes <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
				tick = slowtick
			case svc.Continue:
				changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
				tick = fasttick
			default:
				self.Conf.Log.Println(fmt.Sprintf("unexpected control request #%d", c))
			}
		}
	}
	changes <- svc.Status{State: svc.StopPending}
	return
}