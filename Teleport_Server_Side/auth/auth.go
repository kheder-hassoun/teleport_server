package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
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

/// new jkjdkjkd

// UserUrlDetailsRequest defines the structure for user URL details.
type UserUrlDetailsRequest struct {
	UserName string `json:"userName"`
	Url      string `json:"url"`
	Time     string `json:"time"`
}

// IncrementRequest defines the structure for increment requests.
type IncrementRequest struct {
	UserName string `json:"userName"`
	Url      string `json:"url"`
}

// AddUserUrlDetails sends a request to add user URL details.
func AddUserUrlDetails(apiUrl, token, userName, url, timepass string) error {
	requestPayload := UserUrlDetailsRequest{
		UserName: userName,
		Url:      url,
		Time:     timepass,
	}
	jsonPayload, err := json.Marshal(requestPayload)
	if err != nil {
		return fmt.Errorf("error marshalling JSON: %v", err)
	}

	req, err := http.NewRequest("POST", apiUrl+"/add", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error performing request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	fmt.Println("User URL details added successfully.")
	return nil
}

// SendIncrementRequest sends a request to increment user details.
func SendIncrementRequest(userName, url, apiUrl, token string) error {
	requestPayload := IncrementRequest{
		UserName: userName,
		Url:      url,
	}
	jsonPayload, err := json.Marshal(requestPayload)
	if err != nil {
		return fmt.Errorf("error marshalling JSON: %v", err)
	}

	req, err := http.NewRequest("POST", apiUrl+"/increment", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error performing request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	fmt.Println("Increment API call was successful.")
	return nil
}
