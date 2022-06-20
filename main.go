package main

import (
	"errors"
	"os"
	"os/user"
	"runtime"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/evrrnv/your-map/server/main/src/models"
	"github.com/urfave/cli"
)

var (
	wifiInterface string
	version       string
	commit        string
	date          string
	macSring 	  string
	macList       []string

	server                   string
	family, device, location string

	scanSeconds            int
	minimumThreshold       int
	doBluetooth            bool
	doWifi                 bool
	doReverse              bool
	doDebug                bool
	runForever             bool
	currentChannel string
)

func main() {
	defer log.Flush()
	app := cli.NewApp()
	app.Name = "cli-scanner"
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "bluetooth",
			Usage: "scan bluetooth",
		},
		cli.BoolFlag{
			Name:  "wifi",
			Usage: "scan wifi",
		},
		cli.StringFlag{
			Name:  "server",
			Value: "http://localhost:8005/",
			Usage: "server for submitting fingerprints",
		},
		cli.StringFlag{
			Name:  "interface,i",
			Value: "wlan0",
			Usage: "wifi interface for scanning",
		},
		cli.StringFlag{
			Name:  "location,l",
			Value: "",
			Usage: "location name",
		},
		cli.StringFlag{
			Name:  "device,d",
			Value: "",
			Usage: "device name",
		},
		cli.StringFlag{
			Name:  "sublocation,s",
			Value: "",
			Usage: "sublocation name (automatically toggles learning)",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug mode",
		},
		cli.BoolFlag{
			Name:  "forever",
			Usage: "run until Ctl+C signal",
		},
		cli.IntFlag{
			Name:  "min-rssi",
			Value: -100,
			Usage: "minimum RSSI to use",
		},
		cli.IntFlag{
			Name:  "scantime,t",
			Value: 40,
			Usage: "number of seconds to scan",
		},
		cli.StringFlag{
			Name:  "macs,m",
			Usage: "access points mac addresses",
			FilePath: "./mac-list",
		},
	}
	app.Action = func(c *cli.Context) (err error) {
		server = c.GlobalString("server")
		family = strings.ToLower(c.GlobalString("location"))
		device = c.GlobalString("device")
		wifiInterface = c.GlobalString("interface")
		location = c.GlobalString("sublocation")
		macSring = c.GlobalString("macs")
		doBluetooth = c.GlobalBool("bluetooth")
		doWifi = c.GlobalBool("wifi")
		doDebug = c.GlobalBool("debug")
		runForever = c.GlobalBool("forever")
		scanSeconds = c.GlobalInt("scantime")
		minimumThreshold = c.GlobalInt("min-rssi")
		if doDebug {
			setLogLevel("debug")
		} else {
			setLogLevel("info")
		}

		macList = strings.Split(macSring, ",")

		if runtime.GOOS == "linux" && os.Getenv("SUDO_USER") == "" {
			user, usererr := user.Current()
			if usererr == nil && user.Name != "root" {
				err = errors.New("need to run with sudo")
				return

			}
		}

		if !doBluetooth && !doWifi {
			doWifi = true
		}

		if device == "" {
			return errors.New("device cannot be blank (set with -d)")
		} else if family == "" {
			return errors.New("family cannot be blank (set with -f)")
		}


		for {
			if doWifi {
				log.Infof("scanning with %s", wifiInterface)
			}
			if doBluetooth {
				log.Infof("scanning bluetooth")
			}
			if doBluetooth || doReverse {
				log.Infof("scanning for %d seconds", scanSeconds)
			}

			if !doReverse {
				err = basicCapture()
			}
			if !runForever {
				break
			} else if err != nil {
				log.Warn(err)
			}
		}
		return
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Error(err)
	}

}


func basicCapture() (err error) {
	payload := models.SensorData{}
	payload.Timestamp = time.Now().UnixNano() / int64(time.Millisecond)
	payload.Family = family
	payload.Device = device
	payload.Location = location
	payload.Sensors = make(map[string]map[string]interface{})

	c := make(chan map[string]map[string]interface{})
	numSensors := 0

	if doWifi {
		go scanWifi(c)
		numSensors++
	}

	if doBluetooth {
		go scanBluetooth(c)
		numSensors++
	}

	for i := 0; i < numSensors; i++ {
		data := <-c
		for sensor := range data {
			payload.Sensors[sensor] = make(map[string]interface{})
			for device := range data[sensor] {
				payload.Sensors[sensor][device] = data[sensor][device]
			}
		}
	}

	if len(payload.Sensors) == 0 {
		err = errors.New("collected no data")
		return
	}

	payload.GPS.Latitude = 35.2102962
	payload.GPS.Longitude = -0.633085

	
	err = postData(payload, "/data")
	return
}

