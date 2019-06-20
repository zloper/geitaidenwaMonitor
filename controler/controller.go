package controler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"geitaidenwaMonitor/templates"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ServiceController interface {
	RefreshStatus() AllStatuses
	Status(name string) ServiceStatus
	Restart(name string) error
	Run(name string) error
	Stop(name string) error
	Enable(name string) error  // enable autostart
	Disable(name string) error // disable autostart
	Create(service NewService) error
}

type Conf struct {
	Services []string `json:"services"`
	Global   bool     `json:"global"` // as a system-wide services, otherwise - user based
	location string   `json:"-"`      // config file location
}

func NewServiceControllerByPath(location string) ServiceController {
	jFile, err := ioutil.ReadFile(location)
	if os.IsNotExist(err) {
		// create default
		return &Conf{
			location: location,
		}
	}
	if err != nil {
		panic(err)
	}
	var data Conf
	err = json.Unmarshal(jFile, &data)
	if err != nil {
		panic(err)
	}
	data.location = location

	fmt.Printf("[MONITOR]: Append srv list: %s", &data.Services)
	return &data
}

func NewServiceController() ServiceController {
	return NewServiceControllerByPath(CFG_PATH)
}

func (cfg *Conf) RefreshStatus() AllStatuses {
	res := make([]ServiceStatus, len(cfg.Services))
	for _, srv := range cfg.Services {
		result := cfg.Status(srv)
		fmt.Println(result)
		res = append(res, result)
	}
	return AllStatuses{Services: res}
}

func (cfg *Conf) Status(name string) ServiceStatus {
	result, err := controlQueryField(name, FieldStatus, !cfg.Global)
	if err != nil {
		fmt.Printf("[ERROR]: Status for srv: %s", name)
		return ServiceStatus{Status: StateUnknown, Name: name}
	}
	return ServiceStatus{Status: result, Name: name}
}

func (cfg *Conf) Restart(name string) error {
	_, err := control(name, RESTART, !cfg.Global)
	if err != nil {
		fmt.Printf("[ERROR]: Restart srv: %s", name)
		return err
	}
	return nil
}

func (cfg *Conf) Run(name string) error {
	_, err := control(name, RUN, !cfg.Global)
	if err != nil {
		fmt.Printf("[ERROR]: Run srv: %s", name)
		return err
	}
	return nil
}

func (cfg *Conf) Stop(name string) error {
	_, err := control(name, STOP, !cfg.Global)
	if err != nil {
		fmt.Printf("[ERROR]: Run srv: %s", name)
		return err
	}
	return nil
}

func (cfg *Conf) Create(service NewService) error {
	// resolve working directory
	workingDir, err := filepath.Abs(service.WorkingDirectory)
	if err != nil {
		return err
	}
	service.WorkingDirectory = workingDir
	// generate unit file
	data := &bytes.Buffer{}
	err = templates.ServiceUnitTemplate.Execute(data, service)
	if err != nil {
		return err
	}
	// detect location for unit file
	var location = LocationGlobal
	if !cfg.Global {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		location = filepath.Join(home, LocationUser)
	}
	// ensure that target directory exists
	err = os.MkdirAll(location, 0755)
	if err != nil {
		return err
	}
	unitFile := filepath.Join(location, service.Name+".service")
	// save unit file
	err = ioutil.WriteFile(unitFile, data.Bytes(), 0755)
	if err != nil {
		return err
	}
	// install (enable)
	err = cfg.Enable(service.Name)
	if err != nil {
		return err
	}
	// save to config
	// TODO: maybe save full information
	cfg.Services = append(cfg.Services, service.Name)
	return cfg.save()
}

func (cfg *Conf) Enable(name string) error {
	_, err := control(name, CmdEnable, !cfg.Global)
	return err
}

func (cfg *Conf) Disable(name string) error {
	_, err := control(name, CmdDisable, !cfg.Global)
	return err
}

func (cfg *Conf) save() error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(cfg.location, data, 0755)
}

func control(name string, operation string, user bool) (string, error) {
	stdout := &bytes.Buffer{}
	var args []string
	if user {
		args = append(args, ModeUser)
	}
	args = append(args, operation, name)
	cmd := exec.Command(COMMAND, args...)
	cmd.Stdout = io.Writer(stdout)
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	res := stdout.String()

	return res, nil
}

func controlQueryField(name string, field string, user bool) (string, error) {
	stdout := &bytes.Buffer{}
	var args []string
	if user {
		args = append(args, ModeUser)
	}
	args = append(args, CmdShow, "-p", field, "--value", name)
	cmd := exec.Command(COMMAND, args...)
	cmd.Stdout = io.Writer(stdout)
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	res := strings.TrimSpace(stdout.String())

	return res, nil
}