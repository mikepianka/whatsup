package whatsup

import (
	"fmt"
	"net/http"
)

func checkLink(url string, tries int) int {
	successfulAttempts := 0

	for i := 0; i < tries; i++ {
		resp, err := http.Get(url)
		if err != nil {
			// Error making the request, the endpoint is considered down
			fmt.Printf("Attempt %d: Error - %v\n", i+1, err)
			continue
		}

		// Check the status code
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Attempt %d: Status Code - %d\n", i+1, resp.StatusCode)
			// You may customize the condition based on your needs
			// For example, you might consider non-200 status codes as "down"
			continue
		}

		// The endpoint is up
		fmt.Printf("Attempt %d: Success\n", i+1)
		successfulAttempts++
	}

	// Return the number of successful attempts
	return successfulAttempts
}

func ping(endpoint string, tries int) (string, error) {
	url := "https://example.com"

	successfulAttempts := checkLink(url, tries)

	if successfulAttempts == tries {
		return fmt.Sprintf("%s is up! %d packets transmitted and received", endpoint, tries), nil
	}

	if successfulAttempts > 0 {
		return "", fmt.Errorf("the endpoint is experiencing issues! %d successful attempts out of %d tries", successfulAttempts, tries)
	} else {
		return "", fmt.Errorf("this endpoint is down after %d attempts", tries)
	}
}
