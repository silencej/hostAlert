package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/viper"
	cli "github.com/urfave/cli/v2"

	"golang.org/x/sys/unix"
)

type Target struct {
	Name string `json:"name"` // E.g. slack
	Url  string `json:"url"`
}

type Config struct {
	Target           Target `json:"target"`
	SpaceLimitMB     uint64 `json:"spaceLimitMB"`
	ProcessToMonitor string `json:"processToMonitor"`
}

func readCfg() Config {

	viper.SetConfigName("config") // name of config file (without extension)
	viper.AddConfigPath("$HOME/.hostAlert")
	viper.AddConfigPath(".") // optionally look for config in the working directory
	viper.SetDefault("SpaceLimitMB", 5*1024)
	viper.SetDefault("ProcessToMonitor", "")

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	var content Config
	err = viper.Unmarshal(&content)
	if err != nil {
		log.Panic("Fail to parseConfig: ", err)
	}
	return content
}

func freeSpaceOnUnix() uint64 {
	var stat unix.Statfs_t

	wd, err := os.Getwd()
	if err != nil {
		log.Panic(err)
	}

	unix.Statfs(wd, &stat)

	// Available blocks * size per block = available space in bytes
	freeSpace := stat.Bavail * uint64(stat.Bsize) / 1024 / 1024 // In MB.
	fmt.Println("Free: ", freeSpace, " MB")
	return freeSpace
}

func processIsRunning(process string) (bool, string) {
	out, err := exec.Command("bash", "-c", fmt.Sprintf("ps aux | grep %s | fgrep -v grep", process)).Output()
	if err != nil {
		return false, string(out)
	}

	return true, string(out)
}

type SlackRequest struct {
	Text string `json:"text"`
}

func sendMsg(cfg Config, msg string) {
	log.Println("SendMsg to ", cfg.Target.Name)
	slackReq := SlackRequest{
		Text: msg,
	}
	jsonStr, err := json.Marshal(slackReq)
	if err != nil {
		log.Panic("Fail to marshal json: ", err)
	}
	// log.Println(jsonStr)
	resp, err := http.Post(cfg.Target.Url, "application/json", bytes.NewBuffer(jsonStr))
	if err != nil {
		log.Panic("Fail to post: ", err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Panic("Fail to parse resp: ", err)
	}
	log.Println("Post resp: ", string(body))
}

func main() {

	app := &cli.App{
		Compiled: time.Now(),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Value:   false,
				Usage:   "Force send notification",
			},
		},
		Action: func(c *cli.Context) error {

			freeSpace := freeSpaceOnUnix()
			cfg := readCfg()
			hostname, err := os.Hostname()
			if err != nil {
				log.Panic("Fail to get hostname: ", err)
			}

			process := cfg.ProcessToMonitor
			if process != "" {
				res, out := processIsRunning(process)
				if !res {
					sendMsg(cfg, fmt.Sprintf("%s", out))
				}
			}

			if freeSpace < cfg.SpaceLimitMB {
				sendMsg(cfg, fmt.Sprintf("Not enough space on %s: %d MB free", hostname, freeSpace))
			}

			if c.Bool("force") {
				sendMsg(cfg, fmt.Sprintf("Space on %s: %d MB free", hostname, freeSpace))
				if process != "" {
					res, out := processIsRunning(process)
					sendMsg(cfg, fmt.Sprintf("%t: %s", res, out))
				}

			}
			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}

}
