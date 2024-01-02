package whatsup

import (
	"fmt"
	"net/http"
)

func checkLink(url string, tries int) int {
	successfulAttempts := 0

	for i := 0; i < tries; i++ {
		resp, err := http.Get("https://" + url)
		if err != nil {
			// Error making the request, the endpoint is considered down
			fmt.Printf("Endpoint: %v Attempt %d: Error - %v\n", url, i+1, err)
			continue
		}

		// 403 = forbidden which means server is responding
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusForbidden {
			fmt.Printf("Endpoint: %v Attempt %d: Status Code - %d\n", url, i+1, resp.StatusCode)
			continue
		}

		// The endpoint is up
		fmt.Printf("Endpoint: %v Attempt %d: Success\n", url, i+1)
		successfulAttempts++
	}

	// Return the number of successful attempts
	return successfulAttempts
}

func httpPing(endpoint string, tries int) (string, error) {

	successfulAttempts := checkLink(endpoint, tries)

	if successfulAttempts == tries {
		return fmt.Sprintf("%s is up! %d packets transmitted and received", endpoint, tries), nil
	}

	// handling for if some of the attempts were successful?
	//return "", fmt.Errorf("the endpoint is experiencing issues! %d successful attempts out of %d tries", successfulAttempts, tries)

	return "", fmt.Errorf("endpoint %v is down after %d attempts", endpoint, tries)

}
