package whatsup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// Config holds configuration parameters.
type Config struct {
	TeamsWebhookUrlSuccess string   `json:"teamsWebhookUrlSuccess"`
	TeamsWebhookUrlFailure string   `json:"teamsWebhookUrlFailure"`
	Endpoints              []string `json:"endpoints"`
	Tries                  int      `json:"tries"`
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

// checkEndpoint checks if the provided endpoint is up and writes the result to the provided channel.
func checkEndpoint(endpoint string, tries int, wg *sync.WaitGroup, ch chan<- CheckResult) {
	defer wg.Done()

	// returns a number of tries summary -- not used
	_, err := ping(endpoint, tries)

	if err != nil {
		ch <- CheckResult{endpoint, err, false}
		return
	}

	ch <- CheckResult{endpoint, nil, true}

}

// checkEndpoints asynchronously checks if the provided endpoints are up and returns a slice of the results.
func checkEndpoints(endpoints []string, tries int) []CheckResult {
	var wg sync.WaitGroup
	resultChannel := make(chan CheckResult, len(endpoints))

	for _, ept := range endpoints {
		wg.Add(1)
		go checkEndpoint(ept, tries, &wg, resultChannel)
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
func checkAndSummarizeEndpoints(endpoints []string, tries int) CheckSummary {
	results := checkEndpoints(endpoints, tries)

	downResults, err := filterDownEndpoints(results)

	if err == nil {
		return CheckSummary{AllUp: true, Msg: fmt.Sprintf("All %d endpoints are up.", len(results))}
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
	checkSummary := checkAndSummarizeEndpoints(cfg.Endpoints, cfg.Tries)

	fmt.Println(checkSummary.Msg)

	err := sendSummaryMessageToTeams(cfg.TeamsWebhookUrlSuccess, cfg.TeamsWebhookUrlFailure, checkSummary)

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
