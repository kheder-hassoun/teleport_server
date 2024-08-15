package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

// SignInDto represents the sign-in data
type SignInDto struct {
	UserName string `json:"userName"`
	Password string `json:"password"`
}

// JwtDto represents the JWT response
type JwtDto struct {
	AccessToken      string `json:"accessToken"`
	UserName         string `json:"userName"`
	SubscriptionType string `json:"subscriptionType"`
}

// SignInAndGetSubscriptionType handles signing in and retrieving subscription type
func SignInAndGetSubscriptionType(apiUrl string, signInData SignInDto) (string, error) {
	// Convert the SignInDto struct to JSON
	jsonData, err := json.Marshal(signInData)
	if err != nil {
		return "", fmt.Errorf("error marshaling signInData: %v", err)
	}

	// Create a new HTTP POST request
	req, err := http.NewRequest("POST", apiUrl+"/signin", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	// Set the content type to application/json
	req.Header.Set("Content-Type", "application/json")

	// Send the request using the default HTTP client
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Check if the response status is 200 OK
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("error response from server: %s", string(body))
	}

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %v", err)
	}

	// Unmarshal the response body into a JwtDto struct
	var jwtDto JwtDto
	err = json.Unmarshal(body, &jwtDto)
	if err != nil {
		return "", fmt.Errorf("error unmarshaling response body: %v", err)
	}

	// Return the subscriptionType from the JwtDto struct
	return jwtDto.SubscriptionType, nil
}
