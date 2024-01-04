package whatsup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Config holds configuration parameters.
type Config struct {
	TeamsWebhookUrlSuccess string   `json:"teamsWebhookUrlSuccess"`
	TeamsWebhookUrlFailure string   `json:"teamsWebhookUrlFailure"`
	Endpoints              []string `json:"endpoints"`
	Tries                  int      `json:"tries"`
	Https                  bool     `json:"https"`
}

// CheckResult holds endpoint ping results.
type CheckResult struct {
	Endpoint string
	Err      error
	Up       bool
}

// CheckSummary holds a summary of the ping results of multiple endpoints.
type CheckSummary struct {
	AllUp bool
	Msg   string
}

// checkOS checks that the current runtime OS is mac, linux, or windows.
func checkOS() (string, error) {
	testedOS := []string{"darwin", "linux", "windows"}
	os := runtime.GOOS

	if slices.Contains(testedOS, os) {
		return os, nil
	} else {
		return os, fmt.Errorf("untested OS: %s", os)
	}
}

// checkEndpointPing checks if the provided endpoint is up using the native OS's ping command and writes the result to the provided channel.
func checkEndpointPing(endpoint string, tries int, ch chan<- CheckResult, os string) {
	var triesArg string
	if os == "windows" {
		triesArg = "-n"
	} else {
		triesArg = "-c"
	}

	output, err := exec.Command("ping", endpoint, triesArg, strconv.Itoa(int(tries))).Output()

	if err != nil {
		ch <- CheckResult{endpoint, err, false}
		return
	}

	successOutputLinux := fmt.Sprintf("%d packets transmitted, %d received", tries, tries)
	successOutputMac := fmt.Sprintf("%d packets transmitted, %d packets received", tries, tries)
	successOutputWindows := fmt.Sprintf("    Packets: Sent = %d, Received = %d", tries, tries)

	var successOutput string
	if os == "windows" {
		successOutput = successOutputWindows
	} else if os == "darwin" {
		successOutput = successOutputMac
	} else {
		successOutput = successOutputLinux
	}

	if !strings.Contains(string(output), successOutput) {
		errMsg := fmt.Errorf("%s failed to return all packets", endpoint)
		ch <- CheckResult{endpoint, errMsg, false}
		return
	}

	ch <- CheckResult{endpoint, nil, true}
}

// checkEndpointHttps checks if the provided endpoint is up using a https GET request and writes the result to the provided channel.
func checkEndpointHttps(endpoint string, tries int, ch chan<- CheckResult) {
	successfulAttempts := 0

	for i := 0; i < tries; i++ {
		resp, err := http.Get("https://" + endpoint)

		if err != nil {
			// Error making the request, the endpoint is considered down
			// fmt.Printf("Endpoint: %v Attempt %d: Error - %v\n", endpoint, i+1, err)
			continue
		}

		// 403 = forbidden which means server is responding
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusForbidden {
			// fmt.Printf("Endpoint: %v Attempt %d: Status Code - %d\n", endpoint, i+1, resp.StatusCode)
			continue
		}

		// The endpoint is up
		// fmt.Printf("Endpoint: %v Attempt %d: Success\n", endpoint, i+1)
		successfulAttempts++
	}

	if successfulAttempts != tries {
		errMsg := fmt.Errorf("%s was not up for all %d attempts", endpoint, tries)
		ch <- CheckResult{endpoint, errMsg, false}
		return
	}

	ch <- CheckResult{endpoint, nil, true}
}

// checkEndpoint checks if the provided endpoint is up using either a native OS ping or https request depending on the provided value of https.
func checkEndpoint(endpoint string, tries int, ch chan<- CheckResult, os string, https bool) {
	if https {
		checkEndpointHttps(endpoint, tries, ch)
	} else {
		checkEndpointPing(endpoint, tries, ch, os)
	}
}

// checkEndpoints asynchronously checks if the provided endpoints are up and returns a slice of the results.
func checkEndpoints(endpoints []string, os string, tries int, https bool) []CheckResult {
	var wg sync.WaitGroup
	resultChannel := make(chan CheckResult, len(endpoints))

	for _, ept := range endpoints {
		wg.Add(1)
		go func(ept string) {
			defer wg.Done()
			checkEndpoint(ept, tries, resultChannel, os, https)
		}(ept)
	}

	wg.Wait()
	close(resultChannel)

	var results []CheckResult

	for r := range resultChannel {
		results = append(results, r)
	}

	return results
}

// filterDownEndpoints filters and returns any down endpoints in the provided results.
func filterDownEndpoints(results []CheckResult) ([]CheckResult, error) {
	var downEndpoints []CheckResult

	for _, r := range results {
		if !r.Up {
			downEndpoints = append(downEndpoints, r)
		}
	}

	numDown := len(downEndpoints)

	if numDown == 0 {
		return []CheckResult{}, nil
	} else {
		return downEndpoints, fmt.Errorf("%d endpoints are down", numDown)
	}
}

// checkAndSummarizeEndpoints checks the provided endpoints and returns a summary of their up or down status.
func checkAndSummarizeEndpoints(endpoints []string, os string, tries int, https bool) CheckSummary {
	results := checkEndpoints(endpoints, os, tries, https)

	downResults, err := filterDownEndpoints(results)

	var checkMethod string

	if https {
		checkMethod = "ping"
	} else {
		checkMethod = "HTTPS GET"
	}

	if err == nil {
		return CheckSummary{AllUp: true, Msg: fmt.Sprintf("All %d endpoints are up, and were checked using %s.", len(results), checkMethod)}
	}

	var msg strings.Builder

	msg.WriteString(fmt.Sprintf("**%d endpoints are down!**\n\n\n\n", len(downResults)))

	for _, d := range downResults {
		msg.WriteString(fmt.Sprintf("Endpoint: %s | Error: %s \n\n", d.Endpoint, d.Err))
	}

	return CheckSummary{AllUp: false, Msg: msg.String()}
}

// sendSummaryMessageToTeams sends an endpoint checks summary message to a Microsoft Teams channel via a webhook.
func sendSummaryMessageToTeams(webhookUrlSuccess string, webhookUrlFailure string, checkSummary CheckSummary) error {

	var color, title string
	success := false
	if checkSummary.AllUp {
		color = "#0ac404"
		title = "👍 Endpoints Up"
		success = true
	} else {
		color = "#e81515"
		title = "🔥 ENDPOINTS DOWN"
	}

	// create a Teams message card
	card := map[string]string{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"summary":    title,
		"themeColor": color,
		"title":      title,
		"text":       checkSummary.Msg,
	}

	// marshal the payload to JSON
	data, err := json.Marshal(card)
	if err != nil {
		return err
	}

	// create a new HTTP request
	url := webhookUrlFailure
	if success {
		url = webhookUrlSuccess
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// check if the request was successful
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send message with status code: %d", resp.StatusCode)
	}

	return nil
}

// Sup checks whether the provided endpoints are up or down and then posts a summary message to the provided Teams webhook.
func Sup(cfg Config) error {
	os, err := checkOS()
	if err != nil {
		return err
	}

	checkSummary := checkAndSummarizeEndpoints(cfg.Endpoints, os, cfg.Tries, cfg.Https)

	fmt.Println(checkSummary.Msg)

	err = sendSummaryMessageToTeams(cfg.TeamsWebhookUrlSuccess, cfg.TeamsWebhookUrlFailure, checkSummary)

	if err != nil {
		return fmt.Errorf("error sending message: %v", err)
	}

	return nil
}

// ParseConfig parses the provided config data into a Config struct.
func ParseConfig(data []byte) (Config, error) {
	var cfg Config
	err := json.Unmarshal(data, &cfg)
	return cfg, err
}
