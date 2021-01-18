package main

import (
	"container/list"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	simplejson "github.com/bitly/go-simplejson"
)

// Config is application config struct
type Config struct {
	ClashURL string
	Auth     string
}

var wg sync.WaitGroup
var config Config

func main() {
	configData, _ := ioutil.ReadFile("config.json")
	config := &Config{}
	_ = json.Unmarshal(configData, &config)
	if config.ClashURL == "" {
		linuxVersion, _ := ioutil.ReadFile("/proc/version")
		if strings.Contains(string(linuxVersion), "OpenWrt") {
			fmt.Println("[+] Detect OpenWrt environment, try get openclash config.")

			cnPort := ExecCommand("uci get openclash.config.cn_port")
			cnPort = strings.Trim(cnPort, "\n")
			if cnPort == "" {
				fmt.Println("[-] Error:get cn_port err.")
				os.Exit(-1)
			}
			config.ClashURL = "http://127.0.0.1:" + cnPort

			dashPasswd := ExecCommand("uci get openclash.config.dashboard_password")
			dashPasswd = strings.Trim(dashPasswd, "\n")
			if dashPasswd == "" {
				fmt.Println("[-] Error:get dashPasswd err.")
				os.Exit(-1)
			}
			config.Auth = dashPasswd

			fmt.Println("[+] Found cn_port, dashPasswd:", cnPort+", "+dashPasswd)
		} else {
			fmt.Println("[-] ClashURL is empty, application exiting.")
			os.Exit(-1)
		}
	}

	req, _ := http.NewRequest("GET", config.ClashURL+"/providers/proxies", nil)
	if config.Auth != "" {
		req.Header.Add("Authorization", "Bearer "+config.Auth)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("[-] Get proxies failed! check clash is running and config not error.")
		os.Exit(-1)
	}
	body, _ := ioutil.ReadAll(resp.Body)
	jsonContent := string(body)
	providers, err := simplejson.NewJson([]byte(jsonContent))
	if err != nil {
		fmt.Println("[-] return is not a json, application exiting.")
		os.Exit(-1)
	}
	proxyList, _ := providers.Get("providers").Get("Proxy").Get("proxies").Array()
	var proxyNames = list.New()
	for _, proxy := range proxyList {
		if eachmap, ok := proxy.(map[string]interface{}); ok {
			if (eachmap["type"] != "URLTest") && (eachmap["type"] != "Direct") {
				proxyNames.PushBack(eachmap["name"])
			}
		}
	}

	fmt.Println("[+] Found proxy count:", proxyNames.Len())
	poolSize := runtime.NumCPU()
	runtime.GOMAXPROCS(poolSize)
	ch := make(chan int, poolSize)
	for i := proxyNames.Front(); i != nil; i = i.Next() {
		ch <- 1
		wg.Add(1)
		go request(i.Value.(string), config, client, ch)
	}
	wg.Wait()
	close(ch)

}

// ExecCommand is linux shell helper
func ExecCommand(strCommand string) string {
	cmd := exec.Command("/bin/bash", "-c", strCommand)

	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		fmt.Println("Execute failed when Start:" + err.Error())
		return ""
	}

	outBytes, _ := ioutil.ReadAll(stdout)
	stdout.Close()

	if err := cmd.Wait(); err != nil {
		fmt.Println("Execute failed when Wait:" + err.Error())
		return ""
	}
	return string(outBytes)
}

func request(proxyName string, config *Config, client *http.Client, ch chan int) {
	//fmt.Println(proxyName)
	req, _ := http.NewRequest("GET", config.ClashURL+"/proxies/"+url.PathEscape(proxyName)+"/delay?timeout=5000&url=http:%2F%2Fwww.gstatic.com%2Fgenerate_204", nil)
	if config.Auth != "" {
		req.Header.Add("Authorization", "Bearer "+config.Auth)
	}
	client.Do(req)
	wg.Done()
	<-ch
}
